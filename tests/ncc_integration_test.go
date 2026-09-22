package tests

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
)

// TestNccIntegrationFullScript verifies that an NCC block containing intra-conjunction joins
// and cross-condition parent joins correctly blocks or permits rule instantiation.
func TestNccIntegrationFullScript(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	// A project is certified IF it does NOT have an unresolved high-severity issue
	script := `
(p certify-project
	(project ^id <pid> ^name <pname>)
	-( (issue ^id <iid> ^project-id <pid> ^severity high)
	   (blocker ^issue-id <iid> ^cleared false) )
-->
	(make certified-project ^id <pid> ^name <pname>)
	(write "Project certified:" <pname> (crlf))
)

; Project 1: Has a high-severity issue, but its blocker is cleared true -> Conjunction NOT met -> Certified!
(make project ^id 1 ^name Alpha)
(make issue ^id 101 ^project-id 1 ^severity high)
(make blocker ^issue-id 101 ^cleared true)

; Project 2: Has a high-severity issue WITH an uncleared blocker (false) -> Conjunction MET -> Blocked!
(make project ^id 2 ^name Beta)
(make issue ^id 102 ^project-id 2 ^severity high)
(make blocker ^issue-id 102 ^cleared false)

; Project 3: Has no issues at all -> Conjunction NOT met -> Certified!
(make project ^id 3 ^name Gamma)
`
	runScript(t, eng, script)

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// Projects 1 (Alpha) and 3 (Gamma) must certify; Project 2 (Beta) must be blocked
	if fired != 2 {
		t.Fatalf("expected exactly 2 rule firings, got %d", fired)
	}

	output := out.String()
	t.Logf("Engine output:\n%s", output)
	if !strings.Contains(output, "Project certified: Alpha") {
		t.Errorf("expected Alpha to be certified")
	}
	if !strings.Contains(output, "Project certified: Gamma") {
		t.Errorf("expected Gamma to be certified")
	}
	if strings.Contains(output, "Beta") {
		t.Errorf("Project Beta should NOT have been certified")
	}

	certified := eng.WorkingMemory().FindByClass("certified-project")
	if len(certified) != 2 {
		t.Fatalf("expected 2 certified-project WMEs, got %d", len(certified))
	}
}

// TestNccIntegrationRandomData generates random clusters with random services and incidents,
// verifying that NCC accurately identifies clusters lacking any paired service+incident.
func TestNccIntegrationRandomData(t *testing.T) {
	eng := engine.New()

	ruleSrc := `
(p mark-healthy-cluster
	(cluster ^id <cid>)
	-( (service ^cluster-id <cid> ^name <sname>)
	   (incident ^service-name <sname> ^active true) )
-->
	(make healthy ^cluster-id <cid>)
)
`
	runScript(t, eng, ruleSrc)

	rng := rand.New(rand.NewSource(54321))
	numClusters := 15

	type serviceKey struct {
		clusterID int64
		svcName   string
	}

	var services []serviceKey
	expectedHealthy := make(map[int64]bool)

	// Step 1: Create clusters
	for c := 1; c <= numClusters; c++ {
		cid := int64(c)
		eng.Make("cluster", map[string]model.Value{"id": model.NewInt(cid)})
		expectedHealthy[cid] = true

		// Create 1-4 services per cluster
		numServices := rng.Intn(4) + 1
		for s := 1; s <= numServices; s++ {
			sName := string(rune('A' + c - 1)) + string(rune('0' + s))
			eng.Make("service", map[string]model.Value{
				"cluster-id": model.NewInt(cid),
				"name":       model.NewSymbol(sName),
			})
			services = append(services, serviceKey{clusterID: cid, svcName: sName})
		}
	}

	// Step 2: Randomly assign incidents to some services
	for _, svc := range services {
		// 25% chance of an active incident
		if rng.Float64() < 0.25 {
			eng.Make("incident", map[string]model.Value{
				"service-name": model.NewSymbol(svc.svcName),
				"active":       model.NewSymbol("true"),
			})
			// This cluster now has an active incident on a service -> conjunction satisfied -> NCC blocks!
			delete(expectedHealthy, svc.clusterID)
		}
	}

	t.Logf("Total clusters: %d, Expected healthy (NCC satisfied): %d", numClusters, len(expectedHealthy))

	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != len(expectedHealthy) {
		t.Fatalf("expected %d firings, got %d", len(expectedHealthy), fired)
	}

	healthyWMEs := eng.WorkingMemory().FindByClass("healthy")
	if len(healthyWMEs) != len(expectedHealthy) {
		t.Fatalf("expected %d healthy WMEs, got %d", len(expectedHealthy), len(healthyWMEs))
	}

	for _, w := range healthyWMEs {
		cidVal, _ := w.Get("cluster-id")
		cid := cidVal.Raw().(int64)
		if !expectedHealthy[cid] {
			t.Errorf("cluster %d was not expected to be healthy", cid)
		}
	}
}

// TestNccIntegrationDynamicTransitions verifies the dynamic unblocking and blocking
// lifecycle of NccNode across additions and retractions of sub-conjunction elements.
func TestNccIntegrationDynamicTransitions(t *testing.T) {
	eng := engine.New()
	var out bytes.Buffer
	eng.SetOutputWriter(&out)

	ruleSrc := `
(p monitor-device
	(device ^id <did>)
	-( (component ^device-id <did> ^name <cname>)
	   (fault ^component-name <cname> ^critical yes) )
-->
	(write "DEVICE-OK:" <did> (crlf))
)

(make device ^id 99)
`
	runScript(t, eng, ruleSrc)

	// Step 1: Device exists with 0 components -> Subnetwork empty -> NCC satisfied (1 activation)
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation initially, got %d", eng.ConflictSet().Count())
	}

	// Step 2: Add component C1 -> Conjunction incomplete (no matching fault) -> Still satisfied (1 activation)
	comp1 := eng.Make("component", map[string]model.Value{
		"device-id": model.NewInt(99),
		"name":      model.NewSymbol("power-supply"),
	})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation with component but no fault, got %d", eng.ConflictSet().Count())
	}

	// Step 3: Add fault for non-existent component C2 -> Conjunction incomplete -> Still satisfied
	fNonMatching := eng.Make("fault", map[string]model.Value{
		"component-name": model.NewSymbol("cooling-fan"),
		"critical":       model.NewSymbol("yes"),
	})
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected 1 activation with non-matching fault, got %d", eng.ConflictSet().Count())
	}

	// Step 4: Add matching critical fault for power-supply!
	// Conjunction satisfied (1 match) -> NCC transition 0 -> 1 -> BLOCKED!
	fMatch1 := eng.Make("fault", map[string]model.Value{
		"component-name": model.NewSymbol("power-supply"),
		"critical":       model.NewSymbol("yes"),
	})
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected rule to be blocked when sub-conjunction matches, got %d", eng.ConflictSet().Count())
	}

	// Step 5: Add a second matching fault for power-supply!
	// Transition 1 -> 2 -> Still blocked!
	fMatch2 := eng.Make("fault", map[string]model.Value{
		"component-name": model.NewSymbol("power-supply"),
		"critical":       model.NewSymbol("yes"),
	})
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected rule to stay blocked with 2 submatches, got %d", eng.ConflictSet().Count())
	}

	// Step 6: Retract second fault -> Transition 2 -> 1 -> Still blocked!
	eng.Remove(fMatch2.Timetag)
	if eng.ConflictSet().Count() != 0 {
		t.Fatalf("expected rule to stay blocked with 1 submatch remaining, got %d", eng.ConflictSet().Count())
	}

	// Step 7: Retract first matching fault -> Transition 1 -> 0 -> UNBLOCKED!
	eng.Remove(fMatch1.Timetag)
	if eng.ConflictSet().Count() != 1 {
		t.Fatalf("expected rule to be unblocked (1 activation) after removing last matching fault, got %d", eng.ConflictSet().Count())
	}

	// Step 8: Fire engine
	fired, err := eng.Run(-1)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if fired != 1 {
		t.Fatalf("expected 1 firing after unblocking, got %d", fired)
	}
	if !strings.Contains(out.String(), "DEVICE-OK: 99") {
		t.Errorf("unexpected output: %s", out.String())
	}

	_ = comp1
	_ = fNonMatching
}
