package supervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func createMockNX(t *testing.T, root, version string, includeUF bool) string {
	t.Helper()
	nxDir := filepath.Join(root, "Siemens", "NX"+version)
	ugii := filepath.Join(nxDir, "UGII")
	managed := filepath.Join(ugii, "managed")

	if err := os.MkdirAll(managed, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(ugii, "run_journal.exe"), []byte("mock-journal"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managed, "NXOpen.dll"), []byte("mock-nxopen"), 0644); err != nil {
		t.Fatal(err)
	}
	if includeUF {
		if err := os.WriteFile(filepath.Join(managed, "NXOpen.UF.dll"), []byte("mock-uf"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return nxDir
}

func TestInspectAndDiscover(t *testing.T) {
	tempRoot := t.TempDir()
	nx2512 := createMockNX(t, tempRoot, "2512", true)
	nx2606 := createMockNX(t, tempRoot, "2606", false)

	// Test inspect single
	inst, err := InspectInstallation(nx2512, "test")
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	if inst.Release != "2512" || !inst.HasNXOpenUF {
		t.Fatalf("unexpected inst details: %+v", inst)
	}

	// Test discover with custom roots
	installs, err := Discover(nx2512, nx2606)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(installs) != 2 {
		t.Fatalf("expected 2 installations, got %d", len(installs))
	}

	// Test SelectVersion
	sel2606, err := SelectVersion("2606", nx2512, nx2606)
	if err != nil {
		t.Fatalf("select version failed: %v", err)
	}
	if sel2606.Release != "2606" {
		t.Fatalf("expected 2606, got %s", sel2606.Release)
	}

	// Test Select missing
	if _, err := SelectVersion("9999", nx2512, nx2606); err == nil {
		t.Fatalf("expected error for missing version 9999")
	}
}

func TestInspectInvalidDirectory(t *testing.T) {
	emptyDir := t.TempDir()
	if _, err := InspectInstallation(emptyDir, "test"); err == nil {
		t.Fatalf("expected error on empty non-NX dir")
	}
}
