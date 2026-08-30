package nx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Homiakus/NXGO/internal/supervisor"
)

func TestRealNXSyslogDiscoveryAndStreaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	worker, session := startTestWorker(t, ctx)
	defer worker.Kill()

	// 1. Query session info to get live syslog path
	info, err := session.Info(ctx)
	if err != nil {
		t.Fatalf("session.Info failed: %v", err)
	}

	t.Logf("session.info: release=%s thread=%d syslog=%s", info.Release, info.ThreadID, info.SyslogPath)
	if info.SyslogPath == "" {
		t.Fatalf("expected non-empty syslog_path in session info")
	}

	// 2. Verify syslog file exists on host
	fi, err := os.Stat(info.SyslogPath)
	if err != nil {
		t.Fatalf("stat syslog file %s failed: %v", info.SyslogPath, err)
	}
	if fi.Size() == 0 {
		t.Fatalf("syslog file %s is empty", info.SyslogPath)
	}
	t.Logf("verified active NX syslog on disk: size=%d bytes", fi.Size())

	// 3. Collect recent syslog lines using SyslogCollector
	collector := supervisor.NewSyslogCollector(info.SyslogPath)
	recent, err := collector.ReadRecentLines(4096)
	if err != nil {
		t.Fatalf("ReadRecentLines failed: %v", err)
	}
	t.Logf("recent syslog excerpt (%d bytes):\n%s", len(recent), string(recent[:min(len(recent), 300)]))

	if !strings.Contains(string(recent), "Siemens") && !strings.Contains(string(recent), "NX") && !strings.Contains(string(recent), "UGII") {
		t.Logf("warning: syslog did not contain typical NX keywords, but exists with size %d", fi.Size())
	}

	// 4. Export syslog to per-run artifact directory
	artifactDir := filepath.Join(t.TempDir(), "test_artifacts")
	dest, err := collector.ExportToArtifactDir(artifactDir, "nx_session_syslog.log")
	if err != nil {
		t.Fatalf("ExportToArtifactDir failed: %v", err)
	}
	destInfo, err := os.Stat(dest)
	if err != nil || destInfo.Size() == 0 {
		t.Fatalf("exported syslog artifact missing or empty: %v", err)
	}
	t.Logf("syslog successfully exported to artifact dir: %s (%d bytes)", dest, destInfo.Size())

	_ = worker.Stop(ctx)
}
