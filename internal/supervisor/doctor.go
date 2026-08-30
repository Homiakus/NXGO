package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Homiakus/NXGO/internal/protocol"
)

type CheckStatus string

const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

type CheckResult struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Message string      `json:"message"`
}

type DoctorReport struct {
	OverallPassed bool           `json:"overall_passed"`
	Platform      string         `json:"platform"`
	Protocol      string         `json:"protocol_version"`
	Checks        []CheckResult  `json:"checks"`
	Installations []Installation `json:"installations,omitempty"`
}

func RunDoctor(ctx context.Context, customNXHome string) *DoctorReport {
	report := &DoctorReport{
		OverallPassed: true,
		Platform:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Protocol:      protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor}.String(),
	}

	addCheck := func(name string, status CheckStatus, msg string) {
		if status == StatusFail {
			report.OverallPassed = false
		}
		report.Checks = append(report.Checks, CheckResult{
			Name:    name,
			Status:  status,
			Message: msg,
		})
	}

	// 1. OS check
	if runtime.GOOS == "windows" {
		addCheck("Platform Compatibility", StatusPass, "Running on Windows host")
	} else {
		addCheck("Platform Compatibility", StatusWarn, fmt.Sprintf("Running on %s (real NX execution requires Windows)", runtime.GOOS))
	}

	// 2. Temp directory writable
	tmpDir := os.TempDir()
	testFile := filepath.Join(tmpDir, fmt.Sprintf("nxgo_doctor_test_%d.tmp", os.Getpid()))
	if err := os.WriteFile(testFile, []byte("ok"), 0644); err != nil {
		addCheck("Temp Storage", StatusFail, fmt.Sprintf("Temp dir %s is not writable: %v", tmpDir, err))
	} else {
		_ = os.Remove(testFile)
		addCheck("Temp Storage", StatusPass, fmt.Sprintf("Temp dir %s is writable", tmpDir))
	}

	// 3. Dotnet SDK check
	if _, err := exec.LookPath("dotnet"); err != nil {
		addCheck("DotNet Toolchain", StatusWarn, "dotnet executable not found in PATH")
	} else {
		addCheck("DotNet Toolchain", StatusPass, "dotnet executable found in PATH")
	}

	// 4. NX Discovery
	var roots []string
	if customNXHome != "" {
		roots = append(roots, customNXHome)
	}
	installs, err := Discover(roots...)
	if err != nil {
		addCheck("Siemens NX Installations", StatusWarn, fmt.Sprintf("No NX installations found (%v)", err))
	} else {
		report.Installations = installs
		addCheck("Siemens NX Installations", StatusPass, fmt.Sprintf("Found %d valid NX installation(s)", len(installs)))
	}

	// 5. Invariant Catalog integrity
	if _, err := os.Stat("policy/invariant-compliance.json"); err != nil {
		addCheck("Invariant Policy", StatusWarn, "invariant-compliance.json not found in working directory")
	} else {
		addCheck("Invariant Policy", StatusPass, "invariant compliance map present and accessible")
	}

	return report
}
