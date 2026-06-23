// Package util holds small, dependency-free helpers shared across packages.
package util

import (
	"fmt"
	"path"
	"strings"
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

// underDir reports whether child is inside parent.
func underDir(parent, child string) bool {
	if parent == "/" {
		return true
	}
	return strings.HasPrefix(child, parent+"/")
}
