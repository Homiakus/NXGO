package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Homiakus/NXGO/internal/apiscanner"
	"github.com/Homiakus/NXGO/internal/protocol"
	"github.com/Homiakus/NXGO/internal/supervisor"
)

func main() {
    if err := run(os.Args[1:]); err != nil {
        fmt.Fprintln(os.Stderr, "nxctl:", err)
        os.Exit(1)
    }
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: nxctl <test|installations|doctor|status> [options]")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	switch args[0] {
	case "installations":
		return runInstallations(args[1:])
	case "doctor":
		return runDoctor(ctx, args[1:])
	case "status":
		return runStatus(args[1:])
	case "api":
		return runAPI(ctx, args[1:])
	case "test":
		if len(args) < 2 {
			return errors.New("usage: nxctl test <fast|fuzz|nx|matrix|chaos|soak|perf>")
		}
		return runTest(ctx, args[1])
	default:
		return fmt.Errorf("unknown command %q (supported: test, installations, doctor, status, api)", args[0])
	}
}

func runInstallations(args []string) error {
	asJSON := hasJSONFlag(args)
	installs, err := supervisor.Discover()
	if err != nil {
		if errors.Is(err, supervisor.ErrNoInstallations) {
			if asJSON {
				fmt.Println("[]")
				return nil
			}
			fmt.Println("No Siemens NX installations detected.")
			return nil
		}
		return err
	}

	if asJSON {
		b, _ := json.MarshalIndent(installs, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	fmt.Printf("Discovered %d Siemens NX installation(s):\n", len(installs))
	for i, inst := range installs {
		fmt.Printf("  [%d] Release: %s (Source: %s)\n", i+1, inst.Release, inst.Source)
		fmt.Printf("      Home: %s\n", inst.Home)
		fmt.Printf("      Journal runner: %s\n", inst.RunJournal)
		fmt.Printf("      NXOpen: %s (UF: %t)\n", inst.NXOpenDLL, inst.HasNXOpenUF)
	}
	return nil
}

func runDoctor(ctx context.Context, args []string) error {
	asJSON := hasJSONFlag(args)
	report := supervisor.RunDoctor(ctx, os.Getenv("NXGO_NX_HOME"))

	if asJSON {
		b, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(b))
		if !report.OverallPassed {
			return errors.New("doctor found issues")
		}
		return nil
	}

	fmt.Println("=== NXGO Doctor ===")
	fmt.Printf("Platform: %s | Protocol: %s\n\n", report.Platform, report.Protocol)
	for _, check := range report.Checks {
		fmt.Printf("  [%s] %s: %s\n", check.Status, check.Name, check.Message)
	}
	fmt.Println()
	if len(report.Installations) > 0 {
		fmt.Printf("Detected NX Versions: ")
		var vNames []string
		for _, inst := range report.Installations {
			vNames = append(vNames, inst.Release)
		}
		fmt.Println(strings.Join(vNames, ", "))
	}
	if !report.OverallPassed {
		return errors.New("one or more critical doctor checks failed")
	}
	return nil
}

func runStatus(args []string) error {
	asJSON := hasJSONFlag(args)
	installs, _ := supervisor.Discover()

	statusMap := map[string]any{
		"protocol_version": protocol.Version{Major: protocol.CurrentProtocolMajor, Minor: protocol.CurrentProtocolMinor}.String(),
		"os":               runtime.GOOS,
		"arch":             runtime.GOARCH,
		"nx_installations": len(installs),
	}

	if asJSON {
		b, _ := json.MarshalIndent(statusMap, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	fmt.Println("=== NXGO Status ===")
	fmt.Printf("Protocol Version: %s\n", statusMap["protocol_version"])
	fmt.Printf("Host Environment: %s/%s\n", statusMap["os"], statusMap["arch"])
	fmt.Printf("Configured NX Installations: %d\n", statusMap["nx_installations"])
	return nil
}

func hasJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--json" || a == "-json" {
			return true
		}
	}
	return false
}

func runTest(ctx context.Context, target string) error {
	switch target {
	case "fast":
		if err := runCmd(ctx, "go", "test", "./..."); err != nil { return err }
		if err := runCmd(ctx, "go", "vet", "./..."); err != nil { return err }
		if err := runCmd(ctx, "go", "run", "./cmd/invariantcheck"); err != nil { return err }
		if _, err := exec.LookPath("dotnet"); err != nil {
			return errors.New("dotnet SDK is required by the canonical fast gate because NXGO includes the NX-independent Agent core")
		}
		return runCmd(ctx, "dotnet", "test", "agent/NXGO.Agent.Core.Tests/NXGO.Agent.Core.Tests.csproj", "-c", "Release", "--nologo")
	case "fuzz":
		fuzzTime := strings.TrimSpace(os.Getenv("NXGO_FUZZTIME"))
		if fuzzTime == "" { fuzzTime = "30s" }
		if err := runCmd(ctx, "go", "test", "./internal/objectref", "-run", "^$", "-fuzz", "FuzzReferenceNeverValidAcrossDifferentEpoch", "-fuzztime", fuzzTime); err != nil { return err }
		return runCmd(ctx, "go", "test", "./internal/protocol", "-run", "^$", "-fuzz", "FuzzDecodeFrame", "-fuzztime", fuzzTime)
	case "nx":
		return runRealNX(ctx, os.Getenv("NXGO_NX_HOME"))
	case "matrix":
		raw := strings.TrimSpace(os.Getenv("NXGO_NX_MATRIX"))
		if raw == "" { return errors.New("NXGO_NX_MATRIX is required; use semicolon-separated NX installation roots") }
		for _, home := range strings.Split(raw, ";") {
			if err := runRealNX(ctx, strings.TrimSpace(home)); err != nil { return fmt.Errorf("matrix entry %q: %w", home, err) }
		}
		return nil
	case "chaos":
		return runCmd(ctx, "go", "test", "./internal/fakeagent", "-run", "Chaos", "-count=1")
	case "soak":
		return runCmd(ctx, "go", "test", "./internal/fakeagent", "-run", "Soak", "-count=1")
	case "perf":
		return runCmd(ctx, "go", "test", "./internal/fakeagent", "-run", "^$", "-bench", ".", "-benchmem")
	default:
		return fmt.Errorf("unknown test loop %q", target)
	}
}

func runRealNX(ctx context.Context, home string) error {
	if runtime.GOOS != "windows" { return errors.New("real NX loop requires Windows") }
	if strings.TrimSpace(home) == "" {
		installs, err := supervisor.Discover()
		if err != nil || len(installs) == 0 {
			return errors.New("NXGO_NX_HOME or valid Siemens NX installation is required")
		}
		home = installs[0].Home
	}
	old := os.Getenv("NXGO_NX_HOME")
	if err := os.Setenv("NXGO_NX_HOME", home); err != nil { return err }
	defer os.Setenv("NXGO_NX_HOME", old)

	oldRunReal := os.Getenv("NXGO_RUN_REAL_NX")
	_ = os.Setenv("NXGO_RUN_REAL_NX", "1")
	defer os.Setenv("NXGO_RUN_REAL_NX", oldRunReal)

	if err := runCmd(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/nx-real-smoke.ps1"); err != nil {
		return err
	}
	return runCmd(ctx, "go", "test", "-v", "-timeout", "90s", "./tests/nx")
}

func runWithEnv(ctx context.Context, env []string, name string, args ...string) error {
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Env = append(os.Environ(), env...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin
    if err := cmd.Run(); err != nil { return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err) }
    return nil
}

func runCmd(ctx context.Context, name string, args ...string) error {
    cmd := exec.CommandContext(ctx, name, args...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.Stdin = os.Stdin
    if err := cmd.Run(); err != nil { return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err) }
    return nil
}

func runAPI(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: nxctl api <scan|find|inspect|diff> [options]")
	}
	switch args[0] {
	case "scan":
		return runAPIScan(ctx, args[1:])
	case "find":
		return runAPIFind(args[1:])
	case "inspect":
		return runAPIInspect(args[1:])
	case "diff":
		return runAPIDiff(args[1:])
	default:
		return fmt.Errorf("unknown api subcommand %q (supported: scan, find, inspect, diff)", args[0])
	}
}

func runAPIScan(ctx context.Context, args []string) error {
	var managedDir string
	var outFile string

	for i := 0; i < len(args); i++ {
		if args[i] == "--out" && i+1 < len(args) {
			outFile = args[i+1]
			i++
		} else if !strings.HasPrefix(args[i], "-") {
			managedDir = args[i]
		}
	}

	if managedDir == "" {
		installs, err := supervisor.Discover()
		if err != nil || len(installs) == 0 {
			return errors.New("no Siemens NX installation detected for API scanning; specify path explicitly")
		}
		managedDir = filepath.Dir(installs[0].NXOpenDLL)
	}

	fmt.Printf("Scanning NXOpen managed assemblies from: %s\n", managedDir)
	manifest, err := apiscanner.ScanManagedAssemblies(ctx, managedDir)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	fmt.Printf("Scanned %d exported types across %d assemblies.\n", len(manifest.Types), len(manifest.Assemblies))

	if outFile != "" {
		b, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(outFile, b, 0644); err != nil {
			return fmt.Errorf("failed writing manifest to %s: %w", outFile, err)
		}
		fmt.Printf("API manifest saved to: %s\n", outFile)
	}
	return nil
}

func runAPIFind(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: nxctl api find <query> [--manifest <file.json>]")
	}
	query := args[0]
	manifestFile := "api-manifest.json"
	for i := 0; i < len(args); i++ {
		if args[i] == "--manifest" && i+1 < len(args) {
			manifestFile = args[i+1]
		}
	}

	b, err := os.ReadFile(manifestFile)
	if err != nil {
		return fmt.Errorf("failed reading manifest %s (run 'nxctl api scan --out %s' first): %w", manifestFile, manifestFile, err)
	}

	var manifest apiscanner.APIManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	matches := apiscanner.SearchTypes(&manifest, query)
	fmt.Printf("Found %d matching types for query %q in %s (%s):\n", len(matches), query, manifestFile, manifest.Release)
	for i, m := range matches {
		if i >= 30 {
			fmt.Printf("... and %d more matches\n", len(matches)-30)
			break
		}
		fmt.Printf("  [%s] %s.%s (%d methods, %d properties)\n", m.Kind, m.Namespace, m.Name, len(m.Methods), len(m.Properties))
	}
	return nil
}

func runAPIInspect(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: nxctl api inspect <TypeName> [--manifest <file.json>]")
	}
	typeName := args[0]
	manifestFile := "api-manifest.json"
	for i := 0; i < len(args); i++ {
		if args[i] == "--manifest" && i+1 < len(args) {
			manifestFile = args[i+1]
		}
	}

	b, err := os.ReadFile(manifestFile)
	if err != nil {
		return fmt.Errorf("failed reading manifest %s: %w", manifestFile, err)
	}

	var manifest apiscanner.APIManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	info := apiscanner.InspectType(&manifest, typeName)
	if info == nil {
		return fmt.Errorf("type %q not found in manifest", typeName)
	}

	fmt.Printf("=== %s: %s.%s (%s) ===\n", info.Kind, info.Namespace, info.Name, info.Assembly)
	if len(info.Properties) > 0 {
		fmt.Println("\nProperties:")
		for _, p := range info.Properties {
			fmt.Printf("  %s %s { read: %v, write: %v }\n", p.Type, p.Name, p.CanRead, p.CanWrite)
		}
	}
	if len(info.Methods) > 0 {
		fmt.Println("\nMethods:")
		for _, m := range info.Methods {
			var paramStrs []string
			for _, p := range m.Parameters {
				paramStrs = append(paramStrs, fmt.Sprintf("%s %s", p.Type, p.Name))
			}
			fmt.Printf("  %s %s(%s)\n", m.ReturnType, m.Name, strings.Join(paramStrs, ", "))
		}
	}
	return nil
}

func runAPIDiff(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: nxctl api diff <manifest-a.json> <manifest-b.json>")
	}
	bA, err := os.ReadFile(args[0])
	if err != nil { return fmt.Errorf("read %s: %w", args[0], err) }
	bB, err := os.ReadFile(args[1])
	if err != nil { return fmt.Errorf("read %s: %w", args[1], err) }

	var mA, mB apiscanner.APIManifest
	if err := json.Unmarshal(bA, &mA); err != nil { return err }
	if err := json.Unmarshal(bB, &mB); err != nil { return err }

	diff := apiscanner.DiffManifests(&mA, &mB)
	fmt.Printf("=== API Diff: %s vs %s ===\n", diff.ReleaseA, diff.ReleaseB)
	fmt.Printf("Added Types: %d\n", len(diff.AddedTypes))
	for _, t := range diff.AddedTypes { fmt.Printf("  + [Type] %s\n", t) }
	fmt.Printf("Removed Types: %d\n", len(diff.RemovedTypes))
	for _, t := range diff.RemovedTypes { fmt.Printf("  - [Type] %s\n", t) }
	fmt.Printf("Added Methods: %d\n", len(diff.AddedMethods))
	for _, m := range diff.AddedMethods { fmt.Printf("  + [Method] %s\n", m) }
	fmt.Printf("Removed Methods: %d\n", len(diff.RemovedMethods))
	for _, m := range diff.RemovedMethods { fmt.Printf("  - [Method] %s\n", m) }

	return nil
}

