package supervisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var canonicalRuntimeDLLs = []string{
	"Newtonsoft.Json.dll",
	"NXGO.Protocol.dll",
	"NXGO.Agent.Core.dll",
	"NXGO.Agent.NXHost.dll",
	"NXGO.Agent.NXHost.runtimeconfig.json",
}

func TestResolveWorkerAgentPathsDefaultsToCanonical(t *testing.T) {
	root := createAgentLayout(t, true)
	journal, agentBin, err := resolveWorkerAgentPaths(WorkerConfig{}, root)
	if err != nil {
		t.Fatalf("resolve canonical default: %v", err)
	}
	want := filepath.Join(root, "agent", "bin", "NXGO.Agent.NXHost.dll")
	if journal != want {
		t.Fatalf("canonical target mismatch: got %q want %q", journal, want)
	}
	wantBin, err := filepath.Abs(filepath.Join(root, "agent", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if agentBin != wantBin {
		t.Fatalf("canonical default AgentBin mismatch: got %q want %q", agentBin, wantBin)
	}
}

func TestResolveWorkerAgentPathsCanonicalRequiresCompiledArtifacts(t *testing.T) {
	root := createAgentLayout(t, true)
	journal, agentBin, err := resolveWorkerAgentPaths(WorkerConfig{AgentMode: AgentModeCanonical}, root)
	if err != nil {
		t.Fatalf("resolve canonical mode: %v", err)
	}
	wantTarget := filepath.Join(root, "agent", "bin", "NXGO.Agent.NXHost.dll")
	if journal != wantTarget {
		t.Fatalf("canonical target mismatch: got %q want %q", journal, wantTarget)
	}
	wantBin, err := filepath.Abs(filepath.Join(root, "agent", "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if agentBin != wantBin {
		t.Fatalf("canonical AgentBin mismatch: got %q want %q", agentBin, wantBin)
	}
}

func TestResolveWorkerAgentPathsCanonicalFailsClosedWhenDllMissing(t *testing.T) {
	root := createAgentLayout(t, false)
	_, _, err := resolveWorkerAgentPaths(WorkerConfig{AgentMode: AgentModeCanonical}, root)
	if err == nil {
		t.Fatal("canonical mode unexpectedly accepted missing compiled Agent DLLs")
	}
}

func TestResolveWorkerAgentPathsCanonicalRejectsEachMissingRuntimeDependency(t *testing.T) {
	for _, missing := range canonicalRuntimeDLLs {
		t.Run(missing, func(t *testing.T) {
			root := createAgentLayout(t, true)
			if err := os.Remove(filepath.Join(root, "agent", "bin", missing)); err != nil {
				t.Fatal(err)
			}
			_, _, err := resolveWorkerAgentPaths(WorkerConfig{AgentMode: AgentModeCanonical}, root)
			if err == nil {
				t.Fatalf("canonical mode unexpectedly accepted missing runtime dependency %s", missing)
			}
		})
	}
}

func TestResolveWorkerAgentPathsRejectsAmbiguousCustomJournalAndMode(t *testing.T) {
	root := createAgentLayout(t, true)
	_, _, err := resolveWorkerAgentPaths(WorkerConfig{
		JournalPath: filepath.Join(root, "custom.cs"),
		AgentMode:   AgentModeCanonical,
	}, root)
	if err == nil {
		t.Fatal("WorkerConfig unexpectedly accepted JournalPath plus AgentMode")
	}
}

func TestResolveWorkerAgentPathsAllowsExplicitJournalEscapeHatch(t *testing.T) {
	root := createAgentLayout(t, true)
	custom := filepath.Join(root, "custom.cs")
	journal, agentBin, err := resolveWorkerAgentPaths(WorkerConfig{
		JournalPath: custom,
		AgentBin:    filepath.Join(root, "custom-bin"),
	}, root)
	if err != nil {
		t.Fatalf("explicit journal escape hatch failed: %v", err)
	}
	if journal != custom || agentBin != filepath.Join(root, "custom-bin") {
		t.Fatalf("explicit journal/AgentBin changed unexpectedly: journal=%q bin=%q", journal, agentBin)
	}
}

func TestResolveWorkerAgentPathsRejectsUnknownMode(t *testing.T) {
	root := createAgentLayout(t, true)
	_, _, err := resolveWorkerAgentPaths(WorkerConfig{AgentMode: AgentMode("future")}, root)
	if err == nil {
		t.Fatal("unknown AgentMode unexpectedly accepted")
	}
}

func TestCanonicalWorkerLaunchUsesManagedCoreRunnerAndDLL(t *testing.T) {
	inst := &Installation{
		Home:                `C:\Program Files\Siemens\DesigncenterNX2512`,
		ManagedDir:          `C:\Program Files\Siemens\DesigncenterNX2512\NXBIN\managed_core`,
		RunDotnetCoreNXOpen: `C:\Program Files\Siemens\DesigncenterNX2512\NXBIN\managed_core\run_dotnet_core_nxopen.exe`,
	}
	target := `D:\NXGO\agent\bin\NXGO.Agent.NXHost.dll`
	command, args, err := resolveCanonicalWorkerLaunch(inst, target)
	if err != nil {
		t.Fatalf("resolve managed_core launch: %v", err)
	}
	if command != inst.RunDotnetCoreNXOpen || len(args) != 1 || args[0] != strings.TrimSuffix(target, filepath.Ext(target)) {
		t.Fatalf("unexpected managed_core launch: command=%q args=%q", command, args)
	}
}

func TestCanonicalWorkerLaunchRejectsLegacyManagedRunnerAndNonDLLTarget(t *testing.T) {
	legacy := &Installation{
		Home:                `C:\NX2512`,
		ManagedDir:          `C:\NX2512\NXBIN\managed`,
		RunDotnetCoreNXOpen: `C:\NX2512\NXBIN\managed_core\run_dotnet_core_nxopen.exe`,
	}
	if _, _, err := resolveCanonicalWorkerLaunch(legacy, `D:\agent\NXGO.Agent.NXHost.dll`); err == nil {
		t.Fatal("canonical launch selected legacy managed directory")
	}
	core := *legacy
	core.ManagedDir = `C:\NX2512\NXBIN\managed_core`
	if _, _, err := resolveCanonicalWorkerLaunch(&core, `D:\agent\NXGO.Agent.NXHost.exe`); err == nil {
		t.Fatal("canonical launch accepted an .exe target; runner requires the matching .dll")
	}
}

func TestWorkerJournalStatePathUsesArtifactDirectory(t *testing.T) {
	artifactDir := filepath.Join("e:", "nxgo", "artifacts")
	got := filepath.Join(artifactDir, "request-journal.bin")
	if filepath.Base(got) != "request-journal.bin" || !strings.Contains(got, "artifacts") {
		t.Fatalf("invalid journal state path: %q", got)
	}
}

func createAgentLayout(t *testing.T, withDLLs bool) string {
	t.Helper()
	root := t.TempDir()
	bundle := filepath.Join(root, "agent", "bundle")
	bin := filepath.Join(root, "agent", "bin")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{filepath.Join(bundle, "placeholder.txt")} {
		if err := os.WriteFile(file, []byte("// fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if withDLLs {
		for _, dll := range canonicalRuntimeDLLs {
			if err := os.WriteFile(filepath.Join(bin, dll), []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}
