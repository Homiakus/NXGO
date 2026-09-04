package supervisor_test

import (
	"testing"

	"github.com/Homiakus/NXGO/internal/supervisor"
)

func TestClassifyCrash_AccessViolation(t *testing.T) {
	output := "Fatal error. System.AccessViolationException: Attempted to read or write protected memory.\n   at NXOpen.UF.UFModl.CreateExtrusion()\n"
	syslog := ">>> ERROR: Memory corruption at 0x00007FFB34"

	report := supervisor.ClassifyCrash(output, syslog, -1073741819)
	if report.Kind != supervisor.CrashKindAccessViolation {
		t.Fatalf("expected CrashKindAccessViolation, got %s", report.Kind)
	}
	if report.CulpritSymbol != "NXOpen.UF.UFModl.CreateExtrusion" {
		t.Fatalf("expected culprit symbol NXOpen.UF.UFModl.CreateExtrusion, got %s", report.CulpritSymbol)
	}
	if len(report.SyslogErrors) == 0 {
		t.Fatalf("expected syslog errors in report")
	}
}

func TestClassifyCrash_License(t *testing.T) {
	output := "License not available: SPLM_LICENSE_SERVER port 28000 unreachable"
	syslog := "License checkout failed for gateway"

	report := supervisor.ClassifyCrash(output, syslog, 1)
	if report.Kind != supervisor.CrashKindLicenseExpired {
		t.Fatalf("expected CrashKindLicenseExpired, got %s", report.Kind)
	}
}

func TestClassifyCrash_MissingDLL(t *testing.T) {
	output := "System.DllNotFoundException: Unable to load DLL 'libpart.dll' or one of its dependencies"
	syslog := ""

	report := supervisor.ClassifyCrash(output, syslog, 1)
	if report.Kind != supervisor.CrashKindMissingDLL {
		t.Fatalf("expected CrashKindMissingDLL, got %s", report.Kind)
	}
}
