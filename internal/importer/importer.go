// Package importer ingests the legacy backup script's pipe-delimited tracking
// file (backup_tracking.db) into Archivarr's database, so historical backups
// show up as real source->destination mappings instead of "not backed up".
//
// Each line looks like:
//
//	rel_path|size|mtime|backup_date|dest_label|source_label
//
// for example:
//
//	tv/Show/S01E01.mkv|1362693441|1709383843|2026-05-25|Backup_2TB_Drive_3|UGREEN_Drive_1_12TB
//
// Older files may omit the trailing source_label (5 fields). The importer maps
// every row to a single existing source drive (the one you scanned), finds or
// creates a destination drive per dest_label, and records an 'unverified' backup
// (legacy rows were size-matched, never hash-verified). It is idempotent: the
// UNIQUE(media_item_id, dest_drive_id) constraint plus an existence check mean
// re-running skips rows already imported.
package importer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danbrown95/archivarr/internal/db"
)

// Record is one parsed line of the legacy tracking file.
type Record struct {
	RelPath     string
	Size        int64
	Mtime       int64  // unix seconds
	BackupDate  string // YYYY-MM-DD as written by the old script
	DestLabel   string
	SourceLabel string // empty for older 5-field rows
}

// Options controls an import run.
type Options struct {
	FilePath      string
	SourceDriveID int64 // existing source drive that all rows attach to
	DryRun        bool  // parse and report without writing anything
}

// Stats summarizes an import run.
type Stats struct {
	LinesTotal        int `json:"linesTotal"`
	Parsed            int `json:"parsed"`
	Skipped           int `json:"skipped"` // blank/comment lines
	Errors            int `json:"errors"`  // malformed lines or row failures
	DestDrivesCreated int `json:"destDrivesCreated"`
	MediaMatched      int `json:"mediaMatched"` // linked to an existing media item
	MediaCreated      int `json:"mediaCreated"` // created (not currently on source)
	BackupsInserted   int `json:"backupsInserted"`
	BackupsDuplicate  int `json:"backupsDuplicate"` // already present, skipped
}

// ParseLine parses a single pipe-delimited record. It splits the structured
// trailing fields from the right so a rel_path containing '|' still parses.
func ParseLine(line string) (Record, error) {
	parts := strings.Split(line, "|")
	n := len(parts)
	if n < 5 {
		return Record{}, fmt.Errorf("expected at least 5 fields, got %d", n)
	}

	var r Record
	var sizeStr, mtimeStr string
	if n >= 6 {
		// rel_path may itself contain '|', so it's everything before the last 5.
		r.SourceLabel = parts[n-1]
		r.DestLabel = parts[n-2]
		r.BackupDate = parts[n-3]
		mtimeStr = parts[n-4]
		sizeStr = parts[n-5]
		r.RelPath = strings.Join(parts[:n-5], "|")
	} else { // n == 5: legacy row without a source label
		r.DestLabel = parts[4]
		r.BackupDate = parts[3]
		mtimeStr = parts[2]
		sizeStr = parts[1]
		r.RelPath = parts[0]
	}

	size, err := strconv.ParseInt(strings.TrimSpace(sizeStr), 10, 64)
	if err != nil {
		return Record{}, fmt.Errorf("invalid size %q: %w", sizeStr, err)
	}
	mtime, err := strconv.ParseInt(strings.TrimSpace(mtimeStr), 10, 64)
	if err != nil {
		return Record{}, fmt.Errorf("invalid mtime %q: %w", mtimeStr, err)
	}
	r.Size = size
	r.Mtime = mtime

	if r.RelPath == "" {
		return Record{}, errors.New("empty rel_path")
	}
	if r.DestLabel == "" {
		return Record{}, errors.New("empty destination label")
	}
	return r, nil
}

// parseBackupDate converts the script's YYYY-MM-DD date to unix seconds (UTC),
// returning 0 if it can't be parsed (the mapping matters more than the date).
func parseBackupDate(s string) int64 {
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(s)); err == nil {
		return t.Unix()
	}
	return 0
}

// Import reads opts.FilePath and reconciles it into the database.
func Import(ctx context.Context, d *db.DB, opts Options) (Stats, error) {
	f, err := os.Open(opts.FilePath)
	if err != nil {
		return Stats{}, fmt.Errorf("opening import file: %w", err)
	}
	defer f.Close()

	var st Stats
	destCache := map[string]int64{} // dest label -> drive id (0 = would-create, dry run)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // long paths -> allow long lines
	for sc.Scan() {
		st.LinesTotal++
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			st.Skipped++
			continue
		}

		rec, err := ParseLine(line)
		if err != nil {
			st.Errors++
			continue
		}
		st.Parsed++

		destID, created, err := findOrCreateDest(ctx, d, rec.DestLabel, destCache, opts.DryRun)
		if err != nil {
			st.Errors++
			continue
		}
		if created {
			st.DestDrivesCreated++
		}

		mediaID, isNew, err := resolveMediaItem(ctx, d, opts, rec)
		if err != nil {
			st.Errors++
			continue
		}
		if isNew {
			st.MediaCreated++
		} else {
			st.MediaMatched++
		}

		// A freshly created item can't already have a backup; only check existing.
		if !isNew {
			exists, err := d.BackupExists(ctx, mediaID, destID)
			if err != nil {
				st.Errors++
				continue
			}
			if exists {
				st.BackupsDuplicate++
				continue
			}
		}

		if opts.DryRun {
			st.BackupsInserted++
			continue
		}

		if _, err := d.InsertBackup(ctx, db.InsertBackupInput{
			MediaItemID: mediaID,
			DestDriveID: destID,
			DestRelPath: rec.RelPath, // the old script mirrored the source tree
			Size:        rec.Size,
			Status:      "unverified", // size-matched, never hash-verified
			CopiedAt:    parseBackupDate(rec.BackupDate),
		}); err != nil {
			st.Errors++
			continue
		}
		st.BackupsInserted++
	}
	if err := sc.Err(); err != nil {
		return st, fmt.Errorf("reading import file: %w", err)
	}
	return st, nil
}

// findOrCreateDest returns the destination drive id for a label, creating the
// drive (role=destination, no marker) if it doesn't exist. In dry-run mode it
// reports would-create without writing.
func findOrCreateDest(ctx context.Context, d *db.DB, label string, cache map[string]int64, dryRun bool) (id int64, created bool, err error) {
	if cached, ok := cache[label]; ok {
		return cached, false, nil
	}
	dr, err := d.GetDriveByLabel(ctx, label)
	if err == nil {
		cache[label] = dr.ID
		return dr.ID, false, nil
	}
	if !errors.Is(err, db.ErrDriveNotFound) {
		return 0, false, err
	}
	if dryRun {
		cache[label] = 0 // remember so we don't count it as created twice
		return 0, true, nil
	}
	nd, err := d.CreateDrive(ctx, db.CreateDriveInput{Label: label, Role: db.RoleDestination})
	if err != nil {
		return 0, false, err
	}
	cache[label] = nd.ID
	return nd.ID, true, nil
}

// resolveMediaItem finds the existing media item for this row under the target
// source, or creates one if the file isn't currently tracked (e.g. it was
// deleted from the source since the legacy backup). A later scan reconciles the
// present flag and size/mtime against what's actually on disk.
func resolveMediaItem(ctx context.Context, d *db.DB, opts Options, rec Record) (id int64, created bool, err error) {
	item, err := d.GetMediaItem(ctx, opts.SourceDriveID, rec.RelPath)
	if err == nil {
		return item.ID, false, nil
	}
	if !errors.Is(err, db.ErrMediaItemNotFound) {
		return 0, false, err
	}
	if opts.DryRun {
		return 0, true, nil
	}
	src := opts.SourceDriveID
	id, err = d.InsertMediaItem(ctx, db.InsertMediaItemInput{
		SourceDriveID: &src,
		RelPath:       rec.RelPath,
		Size:          rec.Size,
		Mtime:         rec.Mtime,
	})
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}
