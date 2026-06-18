<p align="center">
  <img src="web/public/icon-mark-1024.png" alt="Archivarr" width="120" />
</p>

<h1 align="center">Archivarr</h1>

<p align="center">
  <strong>Back up your media library to a rotating set of external drives — and always know what's protected, what isn't, and where every copy lives, even with no drives plugged in.</strong>
</p>

Archivarr is a self-hosted, *arr-style companion for media hoarders. It keeps a
persistent database of every file on your source library and which backup drive(s)
each copy lives on. Because that map is stored centrally, you can plan backups and
answer "what did I lose?" **without** the source or destination drives connected.

> **Why?** Large libraries outgrow single backup drives. Archivarr lets you fill
> drive after drive, tracking exactly what's on each — so when a drive dies you
> know precisely which files were lost and which other drives hold the copies.

---

## Features

- 🗂️ **Offline-first tracking** — the database is the source of truth; reason about
  your backups with nothing plugged in.
- 🔁 **Multi-drive rotation** — fill one drive, swap in the next; Archivarr only
  copies what isn't already backed up somewhere.
- 🔗 **Per-file source → destination mapping** — every file knows which drive(s)
  hold its backup, and when it was copied.
- 🧮 **Verified copies** — files are hashed (XXH3) *while* copying, written to a
  temp file and atomically renamed, with mtime/permissions preserved.
- 💽 **Unprivileged drive identity** — destination drives are recognized by a
  marker file regardless of mount path; no privileged container or `/dev` access.
- ⚙️ **Background jobs** — scans and backups run in a worker pool with live
  progress, logs, cancellation, and crash recovery.
- ⏸️ **Pause switch** — disable all automated work (indefinitely or for a set
  time); great for maintenance or testing.
- ⏱️ **Scheduled scans** — automatically keep the library index up to date.
- 🚫 **Include / exclude filters** — skip `.nfo`, artwork, `@eaDir`, etc., or
  track only chosen extensions.
- 📊 **Coverage at a glance** — dashboard + Media page show backed-up vs. pending
  counts/sizes, and per-destination contents.
- 🚑 **Recovery tools** — "a source died: what's lost & where are the copies" and
  "a destination died: re-queue its files for a replacement drive."
- 🎯 **Bulk or single-file backups** — copy everything pending, or just one file.
- 🛟 **Self-protecting** — each backup run drops a snapshot of the tracking DB onto
  the destination drive (`_backup_meta/`).

---

## Quick start (Docker)

Archivarr ships as a single container. Example `docker-compose.yml`:

```yaml
services:
  archivarr:
    build: . # or use a prebuilt image once published (see roadmap)
    container_name: archivarr
    ports:
      - "7979:7979"
    volumes:
      - ./config:/config # SQLite db + settings live here
      - /volume1/docker/media:/media:ro # your source library (READ-ONLY)
      - /mnt:/mnt:rslave # where external backup drives mount on the host
    environment:
      - ARCHIVARR_SCAN_ROOTS=/mnt # where Archivarr looks for backup drives
      - TZ=America/Chicago
    restart: unless-stopped
```

```bash
docker compose up --build
```

Then open <http://localhost:7979>.

1. **Drives → Add source** — point it at your library (e.g. `/media`).
2. **Drives → Discover destinations** — plug in a backup drive and register it
   (Archivarr writes a small `.archivarr-drive-id` marker so it's recognized next
   time, at any mount path).
3. **Media → Scan sources** — index the library.
4. **Drives → Back up** (or per-file from **Media**) — copy what's not yet backed up.

> **Mounting tips.** Mount your source library **read-only** (`:ro`) — Archivarr
> only reads sources. Point `ARCHIVARR_SCAN_ROOTS` (and the matching bind) at
> wherever your NAS mounts external/USB drives. If your whole library lives on one
> volume, add it as **one source** (e.g. `/media`) — it preserves the
> `movies/tv/music` tree on the backup drive and gives you one recovery report.

---

## Configuration

| Env var                       | Default            | Purpose                                              |
| ----------------------------- | ------------------ | ---------------------------------------------------- |
| `ARCHIVARR_PORT`              | `7979`             | HTTP listen port                                     |
| `ARCHIVARR_CONFIG_DIR`        | `/config`          | Holds the SQLite database and logs                   |
| `ARCHIVARR_SCAN_ROOTS`        | `/mnt`             | Comma-separated dirs scanned for backup drives       |
| `ARCHIVARR_MONITOR_INTERVAL`  | `30`               | Seconds between drive online/offline checks          |
| `ARCHIVARR_WORKERS`           | `4`                | Background job worker pool size                      |
| `ARCHIVARR_AUTOMATION_PAUSED` | `false`            | Start with automated work paused (handy for testing) |

Include/exclude patterns and the auto-scan interval are set in the **Settings**
tab of the UI.

---

## How it works

- **Sources** are libraries you want protected (identified by their configured
  path). **Destinations** are rotating external drives (identified by a marker
  file, so they're recognized at any mount path). A drive can be both.
- A **scan** walks a source, recording each file's size + modification time, and
  reconciles new / changed / unchanged / missing files. Content hashing is lazy by
  default (and happens automatically when a file is backed up).
- A **backup** copies not-yet-backed-up files to a destination, hashing during the
  copy, then records the source→destination mapping. When a destination fills up,
  it stops cleanly and resumes on the next drive.
- Everything heavy runs as a cancellable **job** in a worker pool, with one writer
  per physical destination drive at a time.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the full architecture and data model.

---

## Platform

Archivarr targets **Linux** and runs as a Docker container (tested intent: UGREEN,
Synology, TrueNAS, generic Linux + Docker). There is no native Windows build —
develop and test via Docker or a Linux/WSL environment.

- **Backend:** Go (`net/http` + `chi`), SQLite (pure-Go `modernc.org/sqlite`)
- **Frontend:** Vue 3 + Naive UI, embedded into the Go binary via `go:embed`
- **Packaging:** one static binary, one container

## Development

```bash
# Run the full test suite on Linux (no host mounts needed)
docker build --target test .

# Run the app for local dev
docker compose -f compose.local.yml up --build   # serves on :7979

# Frontend hot-reload (backend in Docker, UI via Vite proxy to :7979)
npm --prefix web install
npm --prefix web run dev                          # http://localhost:5173
```

Project layout:

```
cmd/archivarr/     entry point (embeds frontend, starts server + workers)
internal/
  config/          env-based runtime config
  db/              SQLite, migrations, query layer
  drive/           marker-file identity, mount discovery, online monitor
  scan/            filesystem walk + change detection + include/exclude
  hash/            XXH3 content hashing
  jobs/            worker pool, scheduler, job dispatch
  backup/          copy + verify engine
  api/             chi router, REST API, SPA serving
web/               Vue 3 + Naive UI app -> web/dist (embedded)
migrations/        embedded SQL schema migrations
```

## Roadmap

See [`TODO.md`](TODO.md) for planned features (authentication, bitrot detection,
scheduled backups, TMDB metadata, and more). Contributions and ideas welcome.

## Contributing

This is an early, community-oriented project. Issues, feature requests, and PRs
are welcome — please open an issue to discuss larger changes first.

## License

_TBD — a license will be added before/at the first tagged release._
