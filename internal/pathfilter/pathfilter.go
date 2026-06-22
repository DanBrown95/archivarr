// Package pathfilter decides whether a file path is excluded from tracking or
// backup, based on glob exclude patterns plus an optional include-extension
// allowlist. It is shared by the scan, backup, and import flows so they all
// apply identical rules.
package pathfilter

import (
	"path"
	"strings"
)

// Rules holds the include/exclude configuration.
type Rules struct {
	// Exclude are glob patterns matched (case-insensitively) against a file's
	// basename or any path segment; a match means the file is skipped.
	Exclude []string
	// IncludeExt, when non-empty, limits tracking to these extensions (no dot,
	// case-insensitive); anything else is skipped.
	IncludeExt []string
}

// Skip reports whether the slash-relative path rel should be excluded.
func (r Rules) Skip(rel string) bool {
	base := path.Base(rel)
	if len(r.IncludeExt) > 0 {
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(base), "."))
		ok := false
		for _, e := range r.IncludeExt {
			if strings.ToLower(strings.TrimPrefix(e, ".")) == ext {
				ok = true
				break
			}
		}
		if !ok {
			return true
		}
	}
	// Exclude matching is case-insensitive: junk/temp patterns (e.g. *.tmp,
	// .DS_Store, qBittorrent's .!qB) should match regardless of case.
	if len(r.Exclude) > 0 {
		lowerBase := strings.ToLower(base)
		segs := strings.Split(strings.ToLower(rel), "/")
		for _, pat := range r.Exclude {
			lp := strings.ToLower(pat)
			if m, _ := path.Match(lp, lowerBase); m {
				return true
			}
			for _, seg := range segs {
				if m, _ := path.Match(lp, seg); m {
					return true
				}
			}
		}
	}
	return false
}
