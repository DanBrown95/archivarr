<p align="center">
  <img src="web/public/icon-mark-1024.png" alt="Archivarr" width="120" />
</p>

<h1 align="center">Archivarr</h1>

<p align="center">
  <strong>Back up your media library to a rotating set of external drives — and always know what's protected, what isn't, and where every copy lives, even with no drives plugged in.</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-Linux%20%2F%20Docker-blue" alt="Platform: Linux / Docker" />
  <img src="https://img.shields.io/badge/backend-Go%201.25-00ADD8?logo=go&logoColor=white" alt="Backend: Go 1.25" />
  <img src="https://img.shields.io/badge/frontend-Vue%203-42b883?logo=vuedotjs&logoColor=white" alt="Frontend: Vue 3" />
  <img src="https://img.shields.io/badge/PRs-welcome-brightgreen" alt="PRs welcome" />
  <img src="https://img.shields.io/badge/license-TBD-lightgrey" alt="License: TBD" />
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

- **Offline-first tracking** — the database is the source of truth; reason about
  your backups with nothing plugged in.
- **Multi-drive rotation** — fill one drive, swap in the next; Archivarr only
  copies what isn't already backed up somewhere.
- **Per-file source → destination mapping** — every file knows which drive(s)
  hold its backup, and when it was copied.
- **Verified copies** — files are hashed (XXH3) *while* copying, written to a
  temp file and atomically renamed, with mtime/permissions preserved.
- **Unprivileged drive identity** — destination drives are recognized by a
  marker file regardless of mount path; no privileged container or `/dev` access.
- **Background jobs** — scans and backups run in a worker pool with live
  progress, logs, cancellation, and crash recovery.
- **Pause switch** — suspend *scheduled* work (indefinitely or for a set
  time); manual scans/backups you start still run. Great for maintenance or testing.
- **Scheduled scans** — automatically keep the library index up to date.
- **Include / exclude filters** — skip `.nfo`, artwork, `@eaDir`, etc., or
  track only chosen extensions.
- **Coverage at a glance** — dashboard + Media page show backed-up vs. pending
  counts/sizes, and per-destination contents.
- **Recovery tools** — "a source died: what's lost & where are the copies" and
  "a destination died: re-queue its files for a replacement drive."
- **Bulk or single-file backups** — copy everything pending, or just one file.
- **Self-protecting** — each backup run drops a snapshot of the tracking DB onto
  the destination drive (`_backup_meta/`).

---

## Getting started (Docker)

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

> **Versioning.** The app version is derived from git (`git describe`), not edited
> by hand. Build with `./build.sh up --build` to stamp it from the current tag
> (tag a release with `git tag v0.2.0` first); a plain `docker compose up --build`
> reports `dev`. On Windows PowerShell, the equivalent is:
> `$env:VERSION=(git describe --tags --always --dirty); docker compose up --build`.

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
| `ARCHIVARR_LOG_LEVEL`         | `info`             | Log verbosity: `debug`, `info`, `warn`, `error`      |
| `ARCHIVARR_LOG_FORMAT`        | `text`             | Log format for stdout: `text` or `json`              |

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

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the full architecture, tech stack, and
data model.

---

## Support & community

- **Bugs & feature requests:** [GitHub Issues](https://github.com/DanBrown95/archivarr/issues)
  (please search first; one report per issue).
- **Questions, setup help & ideas:** [GitHub Discussions](https://github.com/DanBrown95/archivarr/discussions).
- **What's planned:** the [Roadmap](#roadmap) below and [`TODO.md`](TODO.md).
- **Support the project:** if Archivarr saves you a drive (or a headache), you can
  [sponsor on GitHub](https://github.com/sponsors/DanBrown95) or
  [buy me a coffee](https://www.buymeacoffee.com/danbrown95) — optional and genuinely
  appreciated.

This is an early, community-oriented project — feedback and contributions are
genuinely welcome.

---

## Roadmap

See [`TODO.md`](TODO.md) for planned features (bitrot detection / verify jobs,
scheduled backups, prebuilt images, TMDB metadata, and more).

---

## Contributing

Contributions are welcome — code, docs, testing on real hardware, and ideas all
help. Please read **[`CONTRIBUTING.md`](CONTRIBUTING.md)** for the development
setup and the fork → feature-branch → pull-request workflow, and open an issue to
discuss anything substantial before starting a large PR.

---

## License

_TBD — a license will be added before/at the first tagged release._
