package harness

import (
	"testing"
)

func TestHarnessValidation(t *testing.T) {
	runner := NewRunner()

	tc := &TestCase{
		Name:     "validation_test",
		Strategy: "LEX",
		Source: `
		(p assert-fact
		   (task ^id 1)
		   -->
		   (make fact ^val ok)
		)
		`,
		InitialWM: []WMEAssertion{
			{
				Class:      "task",
				Attributes: map[string]string{"id": "1"},
			},
		},
		ExpectedWM: []WMEAssertion{
			{
				Class:      "fact",
				Attributes: map[string]string{"val": "ok"},
			},
		},
		ForbiddenWM: []WMEAssertion{
			{
				Class:      "error",
				Attributes: map[string]string{"type": "fatal"},
			},
		},
	}

	res := runner.Run(tc)
	if !res.Passed {
		t.Fatalf("expected test case to pass, failed with error: %v", res.Error)
	}

	// Now test failing expected WME
	tcFail := *tc
	tcFail.ExpectedWM = []WMEAssertion{
		{
			Class:      "nonexistent",
			Attributes: map[string]string{"foo": "bar"},
		},
	}
	resFail := runner.Run(&tcFail)
	if resFail.Passed {
		t.Fatalf("expected test case to fail when expected WME is missing")
	}

	// Test forbidden WME detection
	tcForbidden := *tc
	tcForbidden.ForbiddenWM = []WMEAssertion{
		{
			Class:      "fact",
			Attributes: map[string]string{"val": "ok"},
		},
	}
	resForbidden := runner.Run(&tcForbidden)
	if resForbidden.Passed {
		t.Fatalf("expected test case to fail when forbidden WME is present")
	}
}
