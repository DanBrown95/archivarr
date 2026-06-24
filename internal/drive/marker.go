package drive

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/danbrown95/archivarr/internal/util"
)

// markerPath is the drive-identity file: <root>/.archivarr/drive-id. It lives in
// the hidden metadata directory so it survives remounts and is hard to delete by
// accident.
func markerPath(root string) string {
	return filepath.Join(root, util.MetaDirName, util.MarkerFileName)
}

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
	data, err := os.ReadFile(markerPath(root))
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
	if err := os.MkdirAll(filepath.Join(root, util.MetaDirName), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(markerPath(root), []byte(id+"\n"), 0o644); err != nil {
		return "", err
	}
	return id, nil
}
