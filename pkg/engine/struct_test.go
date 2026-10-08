package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/graemenewlands/ops5/pkg/engine"
	"github.com/graemenewlands/ops5/pkg/model"
	"github.com/graemenewlands/ops5/pkg/parser"
	"github.com/graemenewlands/ops5/pkg/wm"
)

type Shipment struct {
	Timetag     int64     `ops5:",timetag"`
	TrackingID  string    `ops5:"tracking_id"`
	Destination string    `ops5:"destination"`
	WeightKG    float64   `ops5:"weight_kg"`
	IsDelivered bool      `ops5:"is_delivered"`
	CreatedAt   time.Time `ops5:"created_at,utc"`
}

func (Shipment) OPS5ClassName() string {
	return "shipment"
}

type DriftedShipment struct {
	TrackingID string `ops5:"tracking_id"`
	WeightKG   int64  `ops5:"weight_kg"` // changed from float64 to int64
}

func (DriftedShipment) OPS5ClassName() string {
	return "shipment"
}

func TestEngine_RegisterStruct_And_MakeFromStruct(t *testing.T) {
	eng := engine.New()
	eng.SetWatchLevel(0)

	// 1. Register schema derived from struct
	schema, err := eng.RegisterStruct(Shipment{})
	if err != nil {
		t.Fatalf("RegisterStruct error: %v", err)
	}
	if schema.Class != "shipment" {
		t.Errorf("expected class 'shipment', got %q", schema.Class)
	}
	if schema.Fingerprint == "" {
		t.Errorf("expected non-empty schema fingerprint")
	}

	// Re-registering identical struct must be idempotent
	schemaAgain, err := eng.RegisterStruct(&Shipment{})
	if err != nil {
		t.Fatalf("re-registering identical struct failed: %v", err)
	}
	if schemaAgain.Fingerprint != schema.Fingerprint {
		t.Errorf("fingerprint mismatch on idempotent registration")
	}

	// 2. Add rule matching the struct's WME
	rules, err := parser.ParseRules(`
		(p process-shipment
		   <s> (shipment ^tracking_id <tid> ^weight_kg > 10.0 ^is_delivered false)
		   -->
		   (modify <s> ^is_delivered true)
		)
	`)
	if err != nil {
		t.Fatalf("ParseRules error: %v", err)
	}
	eng.AddRule(rules[0])

	// 3. Assert WME using MakeFromStruct
	now := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	src := Shipment{
		TrackingID:  "TRK-9001",
		Destination: "Berlin",
		WeightKG:    15.5,
		IsDelivered: false,
		CreatedAt:   now,
	}

	wme, err := eng.MakeFromStruct(src)
	if err != nil {
		t.Fatalf("MakeFromStruct error: %v", err)
	}
	if wme.Class != "shipment" {
		t.Errorf("expected WME class 'shipment', got %q", wme.Class)
	}

	// 4. Run rule engine
	cycles, err := eng.Run(10)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if cycles != 1 {
		t.Fatalf("expected 1 cycle, got %d", cycles)
	}

	// 5. Verify modified WME in working memory and unmarshal back to Go struct
	allWMEs := eng.WorkingMemory().All()
	if len(allWMEs) != 1 {
		t.Fatalf("expected 1 active WME, got %d", len(allWMEs))
	}
	updatedWME := allWMEs[0]

	var result Shipment
	if err := updatedWME.Unmarshal(&result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !result.IsDelivered {
		t.Errorf("expected IsDelivered true after rule execution")
	}
	if result.TrackingID != "TRK-9001" {
		t.Errorf("expected TrackingID TRK-9001, got %q", result.TrackingID)
	}
	if result.Destination != "Berlin" {
		t.Errorf("expected Destination Berlin, got %q", result.Destination)
	}
	if result.WeightKG != 15.5 {
		t.Errorf("expected WeightKG 15.5, got %f", result.WeightKG)
	}
	if !result.CreatedAt.Equal(now) {
		t.Errorf("createdAt mismatch: expected %v, got %v", now, result.CreatedAt)
	}
	if result.Timetag != updatedWME.Timetag {
		t.Errorf("expected timetag %d, got %d", updatedWME.Timetag, result.Timetag)
	}
}

func TestEngine_SchemaDriftDetection(t *testing.T) {
	eng := engine.New()

	// Register original struct
	_, err := eng.RegisterStruct(Shipment{})
	if err != nil {
		t.Fatalf("initial registration failed: %v", err)
	}

	// Attempting to register drifted struct must return error
	_, err = eng.RegisterStruct(DriftedShipment{})
	if err == nil {
		t.Fatalf("expected schema drift error, but got nil")
	}
	if !strings.Contains(err.Error(), "schema drift detected") {
		t.Errorf("expected 'schema drift detected' in error, got %q", err.Error())
	}
}

func TestEngine_SchemaManifest_ExportAndValidate(t *testing.T) {
	eng := engine.New()
	eng.RegisterStruct(Shipment{})

	manifest := eng.SchemaManifest()
	if len(manifest.Schemas) != 1 {
		t.Fatalf("expected 1 schema in manifest, got %d", len(manifest.Schemas))
	}

	summary, ok := manifest.Schemas["shipment"]
	if !ok {
		t.Fatalf("expected 'shipment' in manifest")
	}
	if summary.Fingerprint == "" {
		t.Errorf("expected non-empty fingerprint in summary")
	}

	// Export JSON
	jsonBytes, err := eng.ExportSchemaManifestJSON()
	if err != nil {
		t.Fatalf("ExportSchemaManifestJSON error: %v", err)
	}

	// Parse JSON
	parsed, err := model.ParseSchemaManifestJSON(jsonBytes)
	if err != nil {
		t.Fatalf("ParseSchemaManifestJSON error: %v", err)
	}

	// Validate against engine
	if err := eng.ValidateManifest(parsed); err != nil {
		t.Errorf("ValidateManifest failed: %v", err)
	}

	// Tamper with fingerprint
	tampered := model.NewSchemaManifest()
	tampered.Schemas["shipment"] = model.ClassSchemaSummary{
		Class:       "shipment",
		Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	if err := eng.ValidateManifest(tampered); err == nil {
		t.Errorf("expected validation error on tampered manifest, got nil")
	}
}

func TestPartitionedEngine_RegisterStruct(t *testing.T) {
	router := func(origin, class string, attrs map[string]model.Value) []string {
		return nil
	}
	pe := engine.NewPartitionedEngine(router)

	schema, err := pe.RegisterStruct(Shipment{})
	if err != nil {
		t.Fatalf("pe.RegisterStruct error: %v", err)
	}
	if schema.Fingerprint == "" {
		t.Errorf("expected non-empty fingerprint")
	}

	// Add partition after registration; should inherit schema
	p1, err := pe.AddPartition("p1")
	if err != nil {
		t.Fatalf("AddPartition error: %v", err)
	}

	p1Schema, ok := p1.Engine.GetSchema("shipment")
	if !ok {
		t.Fatalf("expected partition to inherit 'shipment' schema")
	}
	if p1Schema.Fingerprint != schema.Fingerprint {
		t.Errorf("partition schema fingerprint mismatch: %s != %s",
			p1Schema.Fingerprint, schema.Fingerprint)
	}

	// Drift detection on PartitionedEngine
	_, err = pe.RegisterStruct(DriftedShipment{})
	if err == nil {
		t.Fatalf("expected drift error on PartitionedEngine, got nil")
	}
}

func TestWorkingMemory_MakeFromStruct(t *testing.T) {
	memory := wm.New()
	src := Shipment{
		TrackingID:  "WM-123",
		Destination: "Tokyo",
		WeightKG:    8.0,
		IsDelivered: false,
	}

	wme, err := memory.MakeFromStruct(src)
	if err != nil {
		t.Fatalf("MakeFromStruct error: %v", err)
	}
	if wme.Timetag != 1 {
		t.Errorf("expected timetag 1, got %d", wme.Timetag)
	}
	if wme.Class != "shipment" {
		t.Errorf("expected class 'shipment', got %q", wme.Class)
	}

	var dest Shipment
	if err := wme.Unmarshal(&dest); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if dest.TrackingID != "WM-123" || dest.Destination != "Tokyo" || dest.Timetag != 1 {
		t.Errorf("unmarshaled shipment mismatch: %+v", dest)
	}
}
