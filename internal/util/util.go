// Package util holds small, dependency-free helpers and shared constants.
package util

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// On-disk layout for Archivarr's per-destination metadata. Everything lives in a
// single hidden directory at the drive root so it's tidy and hard to delete by
// accident.
const (
	MetaDirName    = ".archivarr"   // hidden metadata directory at a destination's root
	MarkerFileName = "drive-id"     // stable drive identity, inside MetaDirName
	SnapshotName   = "archivarr.db" // tracking-DB snapshot, inside MetaDirName
)

// Bytes formats a byte count in human-readable binary units (e.g. "1.2 GB").
func Bytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// PathsOverlap reports whether two slash-separated paths refer to the same
// location or one contains the other (e.g. "/mnt/Media" and "/mnt/Media/tv").
// Used to stop a source and a backup destination from sharing a location — a
// "backup" on the source provides no protection.
func PathsOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = path.Clean(a), path.Clean(b)
	if a == b {
		return true
	}
	return underDir(a, b) || underDir(b, a)
}

// ResolveSymlinks returns the canonical path with symlinks resolved, falling
// back to the input if it can't be resolved (e.g. the path doesn't exist yet).
// Used before PathsOverlap so a destination symlinked into a source (or vice
// versa) is still detected as a loop.
func ResolveSymlinks(p string) string {
	if p == "" {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return p
}

// underDir reports whether child is inside parent.
func underDir(parent, child string) bool {
	if parent == "/" {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}
