# Architecture

This document describes how Archivarr is built, for contributors. For usage, see
[`README.md`](README.md); for planned work, see [`TODO.md`](TODO.md).

## Philosophy

Archivarr is **offline-first**: the SQLite database is the source of truth for
what exists and where every backup copy lives. Physical drives are things you
*reconcile against* when they're connected — you can plan backups and assess loss
with nothing plugged in.

It ships as a **single static Go binary** with the Vue frontend embedded
(`go:embed`), running as **one unprivileged Docker container**.

## Core concepts

| Concept | Meaning |
| --- | --- |
| **Source** | A library you want protected (e.g. your media folder), identified by its configured root path. Always-on. |
| **Destination** | A rotating external drive that receives backup copies, identified by a marker file so it's recognized at any mount path. |
| **Drive** | A `source`, `destination`, or `both`. Exists in the DB whether or not it's currently mounted. |
| **Media item** | A file on a source: path (relative to the source root), size, mtime, and an optional content hash. |
| **Backup** | A record that a media item was copied to a destination drive, with a verification hash and timestamp. |
| **Job** | A unit of background work (scan / backup) with progress, a log, and a status. |

### Drive identity (and why the container is unprivileged)

Mount paths are unstable (`/mnt/usb1` today, `/mnt/usb3` tomorrow), so Archivarr
does **not** identify drives by path:

- **Destinations** get a small marker file (`.archivarr-drive-id`) written to
  their root. The drive is recognized by that id wherever it mounts.
- **Sources** are identified by their configured root path (always-on).

Because identity is purely filesystem-level, the container needs **no `/dev`
access and no privileged mode** — just a bind mount of where drives appear (e.g.
`/mnt`). Filesystem UUID is captured opportunistically as metadata when readable,
but never required.

## Data model (SQLite)

```
drives        id, label, role(source|destination|both),
              marker_id (destination identity), root_path (source identity),
              fs_uuid (metadata), last_mount_path, capacity_bytes, free_bytes,
              online, last_seen_at, notes, created_at

media_items   id, source_drive_id, rel_path, size, mtime,
              content_hash, hash_algo, present, last_scanned_at
              UNIQUE(source_drive_id, rel_path)

backups       id, media_item_id, dest_drive_id, dest_rel_path, size,
              copied_at, verified_at, verify_hash, status
              UNIQUE(media_item_id, dest_drive_id)

jobs          id, type, status, params_json, progress, stats_json, log,
              created_at, started_at, finished_at

settings      key, value          -- e.g. pause state, scan config (JSON blob)
```

Key derived queries:

- **Pending (not yet backed up):** present `media_items` with no `backups` row.
- **A source drive died — what's recoverable & where:** join `media_items` for
  that source to their `backups` (and the destination labels). Items with no
  backup are unrecoverable.
- **A destination died — re-queue:** delete its `backups` rows; the affected items
  reappear as pending automatically.

## Package layout

```
cmd/archivarr/     entry point: wires config, db, monitor, jobs; embeds frontend
internal/
  config/          env-based runtime config
  db/              open/migrate, query layer (drives, media, backups, jobs, settings, stats)
  drive/           marker-file identity, disk usage, mount discovery, online monitor
  scan/            filesystem walk, size/mtime change detection, include/exclude
  hash/            XXH3 (128-bit) content hashing, streaming + incremental
  jobs/            worker pool, scheduler, per-destination serialization, dispatch
  backup/          copy + verify engine (hash-while-copy, atomic rename, DB snapshot)
  api/             chi router, REST handlers, embedded SPA serving
web/               Vue 3 + Naive UI; built to web/dist and embedded
migrations/        embedded *.sql schema migrations (applied at startup)
```

## How a scan works

1. Resolve the source's root path and walk it.
2. For each file, compute a **quick signature** (size + mtime) and compare to the
   in-memory snapshot of existing rows: new / changed / unchanged / reappeared.
3. Files matching the include/exclude rules are skipped.
4. Rows present last time but not seen now are marked **not present** (they drop
   out of "pending" and the active media view).

Change detection uses size + mtime only — fast even for large libraries. Content
hashing is **lazy by default** (it happens during backup), or eager when
requested.

## How a backup works

1. Resolve the source root and the destination's current mount path (both must be
   online).
2. Select files to copy — everything pending for the source, or a specific list.
3. For each file: check free space; **stream-copy while hashing in one pass**
   (`io.MultiWriter` into a temp file); `fsync`; verify size; preserve mtime;
   **atomically rename** into place. If a prior hash exists and the copied bytes
   don't match, the copy is rejected and the file left pending.
4. Record the `backups` row (with the verification hash).
5. When the destination fills, stop cleanly with remaining-count info (resume on
   the next drive).
6. Write a snapshot of the tracking DB to `<dest>/_backup_meta/archivarr.db` so a
   backup is self-describing even if the database/source is lost.

## Jobs, scheduling, and pause

- A bounded **worker pool** consumes a persistent job queue. Jobs survive process
  restarts (interrupted `running` jobs are recovered; `queued` ones re-enqueued).
- **One writer per physical destination drive** at a time (a keyed mutex), since a
  single spinning disk hates concurrent writes.
- A **scheduler** can enqueue scans for every source on an interval.
- A global **pause** (timed or indefinite) holds jobs `queued` and stops the drive
  monitor — useful for maintenance/testing.

## Notable technical choices

- **Pure-Go SQLite** (`modernc.org/sqlite`) → CGO-free static binary.
- **Rollback journal + single connection** (not WAL): WAL needs shared-memory
  mmap that bind-mounted filesystems don't support, and locks the file
  exclusively (blocking external DB viewers). A single connection serializes
  access and removes lock contention — plenty for a low-traffic, single-user app.
- **XXH3-128** for content hashing — ~10× faster than SHA-256 across large
  libraries, with ample collision resistance for file identity/integrity.
- **Timestamps stored as unix seconds (INTEGER)** for driver-agnostic scanning;
  the API converts to RFC3339.
- **Embedded frontend** (`go:embed`) — one artifact to ship and run.
- **Linux-only** — `DiskUsage` uses `syscall.Statfs`; develop/test via Docker.
