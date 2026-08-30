package supervisor

import (
	"context"
	"testing"
)

func TestDoctorRun(t *testing.T) {
	ctx := context.Background()
	report := RunDoctor(ctx, "")
	if report == nil {
		t.Fatal("doctor report is nil")
	}
	if len(report.Checks) == 0 {
		t.Fatal("doctor reported 0 checks")
	}

	// Verify temp directory check passed
	var tempCheckPassed bool
	for _, c := range report.Checks {
		if c.Name == "Temp Storage" && c.Status == StatusPass {
			tempCheckPassed = true
			break
		}
	}
	if !tempCheckPassed {
		t.Fatalf("expected Temp Storage check to pass: %+v", report.Checks)
	}
}
