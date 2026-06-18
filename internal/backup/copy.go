// Package backup performs the copy + verify work of a backup job: streaming
// files to a destination drive, hashing them in the same pass, and recording
// the source->destination mapping.
package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/danbrown95/archivarr/internal/hash"
)

// TempSuffix marks an in-progress copy so a crash can't leave a partial file
// masquerading as a complete backup.
const TempSuffix = ".archivarr-tmp"

// CopyFile copies src to dest, computing the XXH3 content hash in the same read
// pass. It writes to a temp file, fsyncs, preserves mtime and mode, then
// atomically renames into place. Returns the hex digest and bytes written.
func CopyFile(src, dest string) (hashHex string, size int64, err error) {
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return "", 0, err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", 0, err
	}

	tmp := dest + TempSuffix
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return "", 0, err
	}

	hw := hash.New()
	n, copyErr := io.Copy(io.MultiWriter(out, hw), in)
	if copyErr != nil {
		out.Close()
		os.Remove(tmp)
		return "", 0, copyErr
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return "", 0, err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}

	if n != info.Size() {
		os.Remove(tmp)
		return "", 0, fmt.Errorf("size mismatch: copied %d bytes, source reports %d (changed mid-copy?)", n, info.Size())
	}

	// Preserve modification time so future scans don't see it as changed.
	if err := os.Chtimes(tmp, info.ModTime(), info.ModTime()); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return "", 0, err
	}
	return hw.Hex(), n, nil
}
