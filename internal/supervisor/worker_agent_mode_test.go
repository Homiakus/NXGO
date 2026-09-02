package supervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkerAgentPathsDefaultsToLegacy(t *testing.T) {
	root := createAgentLayout(t, true)
	journal, agentBin, err := resolveWorkerAgentPaths(WorkerConfig{}, root)
	if err != nil {
		t.Fatalf("resolve legacy default: %v", err)
	}
	want := filepath.Join(root, "agent", "bundle", "AgentWorker.cs")
	if journal != want {
		t.Fatalf("legacy journal mismatch: got %q want %q", journal, want)
	}
	if agentBin != "" {
		t.Fatalf("legacy mode unexpectedly selected AgentBin %q", agentBin)
	}
}

func TestResolveWorkerAgentPathsCanonicalRequiresCompiledArtifacts(t *testing.T) {
	root := createAgentLayout(t, true)
	journal, agentBin, err := resolveWorkerAgentPaths(WorkerConfig{AgentMode: AgentModeCanonical}, root)
	if err != nil {
		t.Fatalf("resolve canonical mode: %v", err)
	}
	wantJournal := filepath.Join(root, "agent", "bundle", "CompiledHostBootstrap.cs")
	if journal != wantJournal {
		t.Fatalf("canonical journal mismatch: got %q want %q", journal, wantJournal)
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
	for _, file := range []string{
		filepath.Join(bundle, "AgentWorker.cs"),
		filepath.Join(bundle, "CompiledHostBootstrap.cs"),
	} {
		if err := os.WriteFile(file, []byte("// fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if withDLLs {
		for _, dll := range []string{"NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll"} {
			if err := os.WriteFile(filepath.Join(bin, dll), []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}
