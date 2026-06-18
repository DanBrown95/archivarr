package drive

import (
	"os"
	"path/filepath"
)

// Scanner discovers candidate drives by listing the immediate subdirectories of
// each configured root (e.g. /mnt). Each subdirectory is treated as a mount
// point and probed for a marker file and disk usage.
type Scanner struct {
	Roots []string
}

// Found describes a mount point discovered by a scan.
type Found struct {
	Path      string
	MarkerID  string
	HasMarker bool
	Usage     Usage
}

// Scan returns every mount point found under the configured roots. Roots that
// do not exist (common in local dev) are skipped silently.
func (s Scanner) Scan() ([]Found, error) {
	var out []Found
	for _, root := range s.Roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue // root absent/unreadable — skip
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(root, e.Name())
			id, has, _ := ReadMarker(p)
			usage, _ := DiskUsage(p)
			out = append(out, Found{Path: p, MarkerID: id, HasMarker: has, Usage: usage})
		}
	}
	return out, nil
}
