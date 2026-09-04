package supervisor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Homiakus/NXGO/internal/supervisor"
)

func TestSyslogCollector(t *testing.T) {
	tempDir := t.TempDir()
	dummySyslog := filepath.Join(tempDir, "dummy.syslog")

	content := "Line 1: NX session init\nLine 2: UGII_BASE_DIR=C:\\Siemens\nLine 3: session normal shutdown\n"
	if err := os.WriteFile(dummySyslog, []byte(content), 0644); err != nil {
		t.Fatalf("write dummy syslog: %v", err)
	}

	collector := supervisor.NewSyslogCollector(dummySyslog)

	// 1. Read recent bytes
	bytes, err := collector.ReadRecentLines(1024)
	if err != nil {
		t.Fatalf("ReadRecentLines failed: %v", err)
	}
	if string(bytes) != content {
		t.Fatalf("expected content %q, got %q", content, string(bytes))
	}

	// 2. Export to artifact dir
	artifactDir := filepath.Join(tempDir, "artifacts")
	dest, err := collector.ExportToArtifactDir(artifactDir, "run_syslog.log")
	if err != nil {
		t.Fatalf("ExportToArtifactDir failed: %v", err)
	}

	destData, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read exported syslog failed: %v", err)
	}
	if string(destData) != content {
		t.Fatalf("expected exported content %q, got %q", content, string(destData))
	}
}

func TestSyslogCorrelation(t *testing.T) {
	logData := []byte(`
2026-09-04 12:00:01 [nxgo:session:sess-1] >>> INFO session initialized
2026-09-04 12:00:02 [nxgo:req:req-extrude-1] >>> INFO starting extrude builder
2026-09-04 12:00:03 [nxgo:req:req-extrude-1] [nxgo:tx:tx-99] >>> INFO commit transaction
2026-09-04 12:00:04 [nxgo:req:req-fail-2] >>> ERROR failed to create fillet
`)

	entries := supervisor.ExtractCorrelations(logData)
	if len(entries) != 4 {
		t.Fatalf("expected 4 correlated entries, got %d", len(entries))
	}

	req1Entries := supervisor.FilterByRequestID(entries, "req-extrude-1")
	if len(req1Entries) != 2 {
		t.Fatalf("expected 2 entries for req-extrude-1, got %d", len(req1Entries))
	}

	txEntries := supervisor.FilterByTxID(entries, "tx-99")
	if len(txEntries) != 1 {
		t.Fatalf("expected 1 entry for tx-99, got %d", len(txEntries))
	}
	if txEntries[0].RequestID != "req-extrude-1" {
		t.Fatalf("expected request_id req-extrude-1 on tx entry, got %s", txEntries[0].RequestID)
	}

	failEntries := supervisor.FilterByRequestID(entries, "req-fail-2")
	if len(failEntries) != 1 || failEntries[0].Level != "ERROR" {
		t.Fatalf("expected 1 ERROR entry for req-fail-2, got %+v", failEntries)
	}
}
