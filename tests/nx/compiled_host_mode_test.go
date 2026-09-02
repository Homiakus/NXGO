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

func TestRealNXCanonicalCompiledHostAgentMode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("canonical supervisor mode requires Windows/Siemens NX")
	}
	if os.Getenv("NXGO_RUN_REAL_NX") == "" {
		t.Skip("set NXGO_RUN_REAL_NX=1 or run via the real-NX quality gate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	worker, err := supervisor.StartWorker(ctx, supervisor.WorkerConfig{
		NXHome:         getNXHome(t),
		AgentMode:      supervisor.AgentModeCanonical,
		StartupTimeout: 45 * time.Second,
	})
	if err != nil {
		t.Fatalf("start worker through AgentModeCanonical: %v", err)
	}
	defer worker.Kill()

	if filepath.Base(worker.Config.JournalPath) != "CompiledHostBootstrap.cs" {
		t.Fatalf("canonical mode selected wrong journal: %s", worker.Config.JournalPath)
	}
	if worker.Config.AgentBin == "" {
		t.Fatal("canonical mode did not resolve AgentBin")
	}
	for _, dll := range []string{"NXGO.Agent.Core.dll", "NXGO.Agent.NXHost.dll"} {
		if _, err := os.Stat(filepath.Join(worker.Config.AgentBin, dll)); err != nil {
			t.Fatalf("canonical mode selected incomplete AgentBin: %s: %v", dll, err)
		}
	}
	if worker.Manifest == nil || worker.Manifest.ID == "" {
		t.Fatal("canonical AgentMode worker did not complete handshake")
	}

	session := nxgo.WrapClient(worker.Client, worker.Manifest.ID, 1, worker.Manifest.NXRelease)
	if err := session.Ping(ctx); err != nil {
		t.Fatalf("canonical AgentMode ping failed: %v", err)
	}
	if err := worker.Stop(ctx); err != nil {
		t.Fatalf("canonical AgentMode shutdown failed: %v", err)
	}
	t.Logf("supervisor AgentModeCanonical verified: bootstrap=%s bin=%s release=%s",
		worker.Config.JournalPath, worker.Config.AgentBin, worker.Manifest.NXRelease)
}
