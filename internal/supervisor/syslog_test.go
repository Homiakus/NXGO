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
