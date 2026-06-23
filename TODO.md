# Archivarr Roadmap

Planned work, roughly grouped. Contributions welcome — open an issue to discuss
anything substantial before starting a PR.

## Done (current capabilities)

- Source/destination drives with unprivileged marker-file identity + online monitor
- Filesystem scan with size/mtime change detection and include/exclude filters
- XXH3 content hashing during backup; verified, atomic copies
- Background job queue (scan/backup) with progress, logs, cancel, crash recovery
- Bulk and single-file backups; resume when a destination fills
- Pause switch (timed/indefinite) for all automated work
- Scheduled scans
- Media coverage dashboard + per-destination stats
- Recovery: source-failure report and dead-destination re-queue
- Per-backup snapshot of the tracking DB onto the destination drive
- Web UI (Vue 3 + Naive UI, dark theme)

---

## Authentication & security

- [x] First-run setup that creates a default admin user (app is locked down by default)
- [x] Login / session handling
- [x] Change username and password from the UI
- [x] API key/token for automation (`X-Api-Key` header; dashboards/scripts)
- [ ] ~~(Potentially) multi-user / roles~~

## Integrity — bitrot detection (Verify job)

- [ ] `verify` job type: re-read files on a destination drive, recompute hashes,
      compare to the stored `verify_hash`
- [ ] Mark drift as `failed` (corruption) or `missing` (gone); surface on Media/Recovery
- [ ] Scheduled periodic verification
- [ ] "Last verified" timestamps and per-drive integrity summary

## Automatic / scheduled backups

- [ ] Scheduled backups (not just scans)
- [ ] Destination rotation policy (fill drive X → alert/await next drive)
- [ ] Notify when a drive is full or a backup needs attention
- [x] Better/more logging of automated tasks (structured `slog` job lifecycle + failures to stdout)

## UI polish

- [ ] Live job progress via SSE (replace 2s polling)
- [ ] Richer dashboard (trends, coverage over time)
- [ ] Per-drive detail view (its files, history, integrity)
- [ ] Toast/notification center for job outcomes
- [ ] Mobile-friendly layout pass

## Media metadata & artwork

- [ ] Integrate with TMDB (or another free API) to pull posters/artwork and titles
- [ ] Richer media browsing (by show/movie, with images) instead of raw paths
- [ ] Cache artwork locally; respect API rate limits/keys

## Branding

- [ ] Project logo + favicon (Replace the existing svg mockup placeholder logos with final designs)
- [ ] Header/README artwork and screenshots (UI screenshots still needed)

## Import & visibility

- [x] **Import existing backup drives** — scan a destination that already holds
      backups and register the files that match a current source as existing
      backups, so they aren't re-copied. Matches by relative path; when the drive
      carries an Archivarr DB snapshot, its stored content hashes also match files
      that moved/reorganized on the source. Unmatched files are reported, never
      created (no sources or media are invented).
- [ ] **Per-destination file listing** — browse the exact files stored on a given
      destination drive (currently only counts are shown).
- [ ] Explicit "prune stale entries" action (currently implicit: vanished source
      files are auto-marked not-present)

## Restore

- [ ] Guided restore: given a (replacement) source, copy files back from the
      destination drive(s) that hold them
- [ ] Verify-on-restore

## Release & ops

- [x] CI (gofmt + vet + test + image build on PRs; publish nightly/release on develop/tags)
- [ ] Prebuilt **multi-arch** Docker images (amd64/**arm64**) published to a registry (currently amd64 only)
- [ ] PUID/PGID runtime user remapping (linuxserver-style)
- [x] Apply include/exclude at backup time too (shared `pathfilter` rules)
- [ ] Configurable logging / log retention (level + format configurable; DB job-log retention/pruning still TODO)

---

_Have an idea or want to pick something up? Open an issue._
