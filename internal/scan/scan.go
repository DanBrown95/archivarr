// Package scan walks a source drive's filesystem and reconciles it with the
// media_items table: detecting new, changed, unchanged, reappeared, and missing
// files. Change detection uses a cheap size+mtime "quick signature"; content
// hashing is done lazily by default (or eagerly when HashOnScan is set).
package scan

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
	"github.com/danbrown95/archivarr/internal/hash"
)

// Options tunes a scan.
type Options struct {
	// HashOnScan computes content hashes inline for new/changed files and
	// backfills missing hashes for unchanged files. Slower; off by default.
	HashOnScan bool
	// Exclude are glob patterns matched against a file's basename or any path
	// segment; matching files are not tracked.
	Exclude []string
	// IncludeExt, when non-empty, limits tracking to these extensions (no dot).
	IncludeExt []string
}

// skip reports whether a slash-relative path should be excluded from tracking.
func (o Options) skip(rel string) bool {
	base := path.Base(rel)
	if len(o.IncludeExt) > 0 {
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(base), "."))
		ok := false
		for _, e := range o.IncludeExt {
			if strings.ToLower(strings.TrimPrefix(e, ".")) == ext {
				ok = true
				break
			}
		}
		if !ok {
			return true
		}
	}
	for _, pat := range o.Exclude {
		if m, _ := path.Match(pat, base); m {
			return true
		}
		for _, seg := range strings.Split(rel, "/") {
			if m, _ := path.Match(pat, seg); m {
				return true
			}
		}
	}
	return false
}

// Result summarizes a completed scan.
type Result struct {
	Root       string `json:"root"`
	FilesSeen  int    `json:"filesSeen"`
	New        int    `json:"new"`
	Changed    int    `json:"changed"`
	Unchanged  int    `json:"unchanged"`
	Reappeared int    `json:"reappeared"`
	Missing    int    `json:"missing"`
	Hashed     int    `json:"hashed"`
	BytesSeen  int64  `json:"bytesSeen"`
	Errors     int    `json:"errors"`
}

// Engine runs scans against the database.
type Engine struct {
	DB *db.DB
}

// ScanSource walks the source drive's root path, updating media_items.
func (e *Engine) ScanSource(ctx context.Context, drive *db.Drive, opts Options) (*Result, error) {
	if drive.RootPath == nil || *drive.RootPath == "" {
		return nil, fmt.Errorf("drive %d has no root path to scan", drive.ID)
	}
	root := *drive.RootPath
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("source root not accessible: %s", root)
	}

	// Load the current state of this source into memory for fast diffing and
	// in-memory missing-detection (no per-file SELECT, no touch-writes).
	existingList, err := e.DB.ListSourceItems(ctx, drive.ID)
	if err != nil {
		return nil, fmt.Errorf("loading existing items: %w", err)
	}
	existing := make(map[string]db.MediaItem, len(existingList))
	for _, m := range existingList {
		existing[m.RelPath] = m
	}
	seen := make(map[string]bool, len(existingList))

	scanTime := time.Now().Unix()
	res := &Result{Root: root}

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			res.Errors++
			return nil // skip unreadable entry, keep going
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			res.Errors++
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			res.Errors++
			return nil
		}
		rel = filepath.ToSlash(rel)
		if opts.skip(rel) {
			return nil // excluded by include/exclude rules
		}
		seen[rel] = true

		size := info.Size()
		mtime := info.ModTime().Unix()
		res.FilesSeen++
		res.BytesSeen += size

		prev, known := existing[rel]
		switch {
		case !known:
			e.handleNew(ctx, drive.ID, rel, path, size, mtime, scanTime, opts, res)
		case prev.Size != size || prev.Mtime != mtime:
			e.handleChanged(ctx, prev, path, size, mtime, scanTime, opts, res)
		default:
			e.handleUnchanged(ctx, prev, path, opts, res)
		}
		return nil
	})
	if walkErr != nil {
		return res, walkErr // typically ctx cancellation
	}

	// Anything previously present but not seen this pass is now missing.
	for rel, m := range existing {
		if m.Present && !seen[rel] {
			if err := e.DB.SetMediaItemPresent(ctx, m.ID, false); err != nil {
				res.Errors++
				continue
			}
			res.Missing++
		}
	}

	return res, nil
}

func (e *Engine) handleNew(ctx context.Context, sourceID int64, rel, path string, size, mtime, scanTime int64, opts Options, res *Result) {
	var hashPtr, algoPtr *string
	if opts.HashOnScan {
		if h, err := hash.File(path); err != nil {
			res.Errors++
		} else {
			algo := hash.Algo
			hashPtr, algoPtr = &h, &algo
			res.Hashed++
		}
	}
	if _, err := e.DB.InsertMediaItem(ctx, db.InsertMediaItemInput{
		SourceDriveID: &sourceID,
		RelPath:       rel,
		Size:          size,
		Mtime:         mtime,
		ContentHash:   hashPtr,
		HashAlgo:      algoPtr,
		ScanTime:      scanTime,
	}); err != nil {
		res.Errors++
		return
	}
	res.New++
}

func (e *Engine) handleChanged(ctx context.Context, prev db.MediaItem, path string, size, mtime, scanTime int64, opts Options, res *Result) {
	var hashPtr, algoPtr *string
	if opts.HashOnScan {
		if h, err := hash.File(path); err != nil {
			res.Errors++
		} else {
			algo := hash.Algo
			hashPtr, algoPtr = &h, &algo
			res.Hashed++
		}
	}
	// A nil hash here intentionally clears the now-stale content hash.
	if err := e.DB.UpdateMediaItemContent(ctx, prev.ID, size, mtime, hashPtr, algoPtr, scanTime); err != nil {
		res.Errors++
		return
	}
	res.Changed++
	if !prev.Present {
		// File came back at source (and changed) after having gone missing.
		res.Reappeared++
	}
}

func (e *Engine) handleUnchanged(ctx context.Context, prev db.MediaItem, path string, opts Options, res *Result) {
	res.Unchanged++
	if !prev.Present {
		// File reappeared at source after having gone missing.
		if err := e.DB.SetMediaItemPresent(ctx, prev.ID, true); err != nil {
			res.Errors++
		} else {
			res.Reappeared++
		}
	}
	if opts.HashOnScan && prev.ContentHash == nil {
		if h, err := hash.File(path); err != nil {
			res.Errors++
		} else if err := e.DB.SetMediaItemHash(ctx, prev.ID, h, hash.Algo); err != nil {
			res.Errors++
		} else {
			res.Hashed++
		}
	}
}
