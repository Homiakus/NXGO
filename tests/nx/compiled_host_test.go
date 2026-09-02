package nx_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/supervisor"
	"github.com/Homiakus/NXGO/pkg/nxgo"
)

// TestRealNXCanonicalCompiledHost proves the H4 migration lane end-to-end:
// run_journal -> minimal CompiledHostBootstrap.cs -> NXGO.Agent.NXHost.dll ->
// NXGO.Agent.Core.dll -> shared NxExecutor on the NX execution thread.
//
// The self-hosted real-NX workflow sets NXGO_REQUIRE_COMPILED_HOST=1, making a
// missing build output a hard failure. Ad-hoc local real-NX runs may skip this
// one fixture until scripts/build-agent.ps1 has been executed.
func TestRealNXCanonicalCompiledHost(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("canonical compiled NXHost requires Windows/Siemens NX")
	}
	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("set NXGO_RUN_REAL_NX=1 or run via the real-NX quality gate")
	}

	repoRoot := repoRootFromTestFile(t)
	agentBin := os.Getenv("NXGO_AGENT_BIN")
	if agentBin == "" {
		agentBin = filepath.Join(repoRoot, "agent", "bin")
	}

	coreDLL := filepath.Join(agentBin, "NXGO.Agent.Core.dll")
	hostDLL := filepath.Join(agentBin, "NXGO.Agent.NXHost.dll")
	missing := ""
	if _, err := os.Stat(coreDLL); err != nil {
		missing = coreDLL
	}
	if _, err := os.Stat(hostDLL); err != nil {
		missing = hostDLL
	}
	if missing != "" {
		if os.Getenv("NXGO_REQUIRE_COMPILED_HOST") == "1" {
			t.Fatalf("canonical compiled Agent output is mandatory but missing: %s; run scripts/build-agent.ps1", missing)
		}
		t.Skipf("canonical compiled Agent not built: %s", missing)
	}

	oldBin, hadBin := os.LookupEnv("NXGO_AGENT_BIN")
	if err := os.Setenv("NXGO_AGENT_BIN", agentBin); err != nil {
		t.Fatalf("set NXGO_AGENT_BIN: %v", err)
	}
	defer func() {
		if hadBin {
			_ = os.Setenv("NXGO_AGENT_BIN", oldBin)
		} else {
			_ = os.Unsetenv("NXGO_AGENT_BIN")
		}
	}()

	bootstrap := filepath.Join(repoRoot, "agent", "bundle", "CompiledHostBootstrap.cs")
	if _, err := os.Stat(bootstrap); err != nil {
		t.Fatalf("compiled-host bootstrap missing: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	worker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{
		NXHome:         getNXHome(t),
		JournalPath:    bootstrap,
		StartupTimeout: 45 * time.Second,
	})
	if err != nil {
		t.Fatalf("start canonical compiled NXHost: %v", err)
	}
	defer worker.Kill()

	if worker.Manifest == nil || worker.Manifest.ID == "" {
		t.Fatal("canonical compiled NXHost did not produce a session manifest")
	}

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("canonical compiled NXHost ping failed: %v", err)
	}
	info, err := session.Info(ctx)
	if err != nil {
		t.Fatalf("canonical compiled NXHost session.info failed: %v", err)
	}
	if info.ThreadID == 0 {
		t.Fatal("canonical compiled NXHost returned zero NX execution thread id")
	}
	if info.SessionID != worker.Manifest.ID {
		t.Fatalf("session identity mismatch: info=%q manifest=%q", info.SessionID, worker.Manifest.ID)
	}
	if info.Epoch != 1 {
		t.Fatalf("unexpected canonical NXHost epoch: got %d want 1", info.Epoch)
	}

	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("canonical compiled NXHost shutdown failed: %v", err)
	}
	t.Logf("canonical Core->NXHost path verified: session=%s thread=%d release=%s", info.SessionID, info.ThreadID, info.Release)
}

func repoRootFromTestFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve compiled_host_test.go path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolved repository root is invalid: %s: %v", root, err)
	}
	return root
}
