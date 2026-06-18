package drive_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danbrown95/archivarr/internal/drive"
)

func TestEnsureMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()

	id, err := drive.EnsureMarker(dir)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if id == "" {
		t.Fatalf("empty marker id")
	}

	got, ok, err := drive.ReadMarker(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !ok || got != id {
		t.Fatalf("read marker = %q ok=%v, want %q true", got, ok, id)
	}

	// EnsureMarker is idempotent: same id on subsequent calls.
	again, err := drive.EnsureMarker(dir)
	if err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if again != id {
		t.Fatalf("marker changed: %q -> %q", id, again)
	}
}

func TestReadMarkerAbsent(t *testing.T) {
	_, ok, err := drive.ReadMarker(t.TempDir())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatalf("expected no marker in empty dir")
	}
}

func TestDiskUsage(t *testing.T) {
	u, err := drive.DiskUsage(t.TempDir())
	if err != nil {
		t.Fatalf("disk usage: %v", err)
	}
	if u.CapacityBytes == 0 {
		t.Fatalf("expected nonzero capacity")
	}
}

func TestScannerDetectsMarkedDrive(t *testing.T) {
	root := t.TempDir()

	marked := filepath.Join(root, "drive1")
	if err := os.Mkdir(marked, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := drive.EnsureMarker(marked)
	if err != nil {
		t.Fatal(err)
	}

	unmarked := filepath.Join(root, "drive2")
	if err := os.Mkdir(unmarked, 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := drive.Scanner{Roots: []string{root}}.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d mounts, want 2", len(found))
	}

	byPath := map[string]drive.Found{}
	for _, f := range found {
		byPath[f.Path] = f
	}
	if f := byPath[marked]; !f.HasMarker || f.MarkerID != id {
		t.Fatalf("marked drive not detected: %+v", f)
	}
	if f := byPath[unmarked]; f.HasMarker {
		t.Fatalf("unmarked drive should have no marker: %+v", f)
	}
}

func TestScannerSkipsMissingRoot(t *testing.T) {
	found, err := drive.Scanner{Roots: []string{filepath.Join(t.TempDir(), "does-not-exist")}}.Scan()
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("expected no mounts, got %d", len(found))
	}
}
