package drive

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MarkerFileName is written to a destination drive's root to give it a stable
// identity that survives remounts at different paths.
const MarkerFileName = ".archivarr-drive-id"

// newMarkerID returns a random 128-bit hex identity.
func newMarkerID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ReadMarker returns the marker id stored at root, with ok=false if no marker
// file is present (or it is empty).
func ReadMarker(root string) (id string, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(root, MarkerFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	id = strings.TrimSpace(string(data))
	if id == "" {
		return "", false, nil
	}
	return id, true, nil
}

// EnsureMarker returns the existing marker id at root, creating one if absent.
func EnsureMarker(root string) (string, error) {
	if id, ok, err := ReadMarker(root); err != nil {
		return "", err
	} else if ok {
		return id, nil
	}
	id, err := newMarkerID()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(root, MarkerFileName), []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}
