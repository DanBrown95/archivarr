package hash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danbrown95/archivarr/internal/hash"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFileStableAndDistinct(t *testing.T) {
	a := writeFile(t, "hello world")
	b := writeFile(t, "hello worlds") // one byte different

	h1, err := hash.File(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(h1) != 32 {
		t.Fatalf("expected 32 hex chars (128-bit), got %d (%q)", len(h1), h1)
	}

	h1again, _ := hash.File(a)
	if h1 != h1again {
		t.Fatalf("hash not stable: %q vs %q", h1, h1again)
	}

	h2, _ := hash.File(b)
	if h1 == h2 {
		t.Fatalf("different content produced same hash")
	}
}

func TestReaderMatchesFile(t *testing.T) {
	content := "the quick brown fox"
	p := writeFile(t, content)

	fromFile, _ := hash.File(p)
	fromReader, err := hash.Reader(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if fromFile != fromReader {
		t.Fatalf("File and Reader disagree: %q vs %q", fromFile, fromReader)
	}
}

func TestEmptyFile(t *testing.T) {
	p := writeFile(t, "")
	h, err := hash.File(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 32 {
		t.Fatalf("expected 32 hex chars for empty file, got %q", h)
	}
}
