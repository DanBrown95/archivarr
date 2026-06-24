# Archivarr Roadmap

Planned work, roughly grouped. Contributions welcome — open an issue to discuss
anything substantial before starting a PR.

## Integrity — bitrot detection (verify job)

- [ ] `verify` job type: re-read files on a destination, recompute hashes, and
      compare them to the stored `verify_hash`
- [ ] Mark drift as `failed` (corruption) or `missing` (gone); surface it on the
      Media / Recovery pages
- [ ] Scheduled periodic verification
- [ ] "Last verified" timestamps and a per-drive integrity summary

## Automatic / scheduled backups

- [ ] Scheduled backups (not just scans)
- [ ] Destination rotation policy (fill drive X → alert / await the next drive)
- [ ] Notify when a drive is full or a backup needs attention

## Restore

- [ ] Guided restore: given a (replacement) source, copy files back from the
      destination drive(s) that hold them
- [ ] Verify-on-restore

## Import & visibility

- [ ] Per-destination file listing — browse the exact files stored on a given
      destination drive (currently only counts are shown)
- [ ] Explicit "prune stale entries" action (currently implicit: vanished source
      files are auto-marked not-present)
- [ ] Content-hash fallback for the *filesystem* import, so files reorganized on a
      destination can be re-matched by content (would read/hash destination files)

## UI polish

- [ ] Live job progress via SSE (replace the 2s polling)
- [ ] Richer dashboard (trends, coverage over time)
- [ ] Per-drive detail view (its files, history, integrity)
- [ ] Toast / notification center for job outcomes
- [ ] Mobile-friendly layout pass

## Media metadata & artwork

- [ ] Integrate with TMDB (or another free API) for posters / artwork and titles
- [ ] Richer media browsing (by show / movie, with images) instead of raw paths
- [ ] Cache artwork locally; respect API rate limits / keys

## Branding

- [ ] Final logo + favicon (replace the placeholder SVG mockups)
- [ ] Header / README artwork and UI screenshots

## Release & ops

- [ ] Prebuilt multi-arch images (amd64 **and** arm64) — currently amd64 only
- [ ] PUID / PGID runtime user remapping (linuxserver-style)
- [ ] Log retention / pruning of the per-job DB log (level + format are already configurable)

## Performance (fine for typical libraries; revisit for very large ones)

- [ ] Batch DB writes in scan / backup / import into transactions (today the single
      SQLite connection does one fsync per file — slow at 100k+ files)
- [ ] Preload a destination's existing backups into a set instead of a per-file
      lookup during import (N+1)
- [ ] Stream source items instead of loading the whole list into memory
- [ ] Cheaper stats queries; faster Media search (avoid the leading-wildcard `LIKE`)

## Known limitations

- **Imported drives must mirror the source tree at the drive root.** Filesystem
  import matches by relative path, so a pre-existing manual backup nested under an
  extra folder (e.g. `media/…` or `backup/media/…`) won't match — those files show
  as not-backed-up. Workaround: place the backup at the drive root. Possible future
  improvement: an optional "backups live under this subfolder" import setting, or
  content-hash matching (see Import & visibility above).
- **Single account only.** Multi-user / roles aren't supported by design for now
  (the schema leaves room to add it later).

---

_Have an idea or want to pick something up? Open an issue._
