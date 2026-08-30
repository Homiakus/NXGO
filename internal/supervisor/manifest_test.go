package supervisor

import (
	"path/filepath"
	"testing"
	"time"
)

func TestManifestSaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "sub", "worker-manifest.json")

	manifest := &WorkerManifest{
		ID:          "worker-100",
		PID:         12345,
		NXHome:      "C:\\Siemens\\NX2512",
		NXRelease:   "2512",
		Endpoint:    `\\.\pipe\nxgo-100`,
		StartedAt:   time.Now().UTC().Truncate(time.Second),
		Owner:       "test-user",
		Mode:        "worker",
		ArtifactDir: "C:\\Temp\\nxgo-artifacts",
	}

	if err := SaveManifest(manifestPath, manifest); err != nil {
		t.Fatalf("save manifest failed: %v", err)
	}

	loaded, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest failed: %v", err)
	}

	if loaded.ID != manifest.ID || loaded.PID != manifest.PID || loaded.NXRelease != "2512" {
		t.Fatalf("loaded manifest mismatch: got %+v, want %+v", loaded, manifest)
	}
}

func TestManifestValidation(t *testing.T) {
	invalid := &WorkerManifest{
		ID: "",
	}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected error on invalid manifest")
	}
}

