# Contributing to Archivarr

First off — thanks for taking the time to contribute! Archivarr is an early,
community-oriented project, and help of every kind is welcome:

- **Code** — bug fixes and features (see the [Roadmap](TODO.md)).
- **Testing** — exercising Archivarr against real drives and NAS hardware and
  reporting what breaks is genuinely valuable.
- **Docs** — improving this guide, the [README](README.md), or
  [ARCHITECTURE.md](ARCHITECTURE.md).
- **Ideas** — open a [Discussion](https://github.com/DanBrown95/archivarr/discussions)
  to float a feature before it becomes a PR.

This project follows the conventions of the wider *arr / Servarr community
(Sonarr, Radarr, etc.), adapted for a Go + Vue codebase.

---

## Reporting bugs

Open a [GitHub Issue](https://github.com/DanBrown95/archivarr/issues). Before you do:

- **Search first** — someone may have already reported it.
- **One issue per bug.**
- Include: what you did, what you expected, what actually happened, your deployment
  (Docker image/tag or commit), relevant **logs**, and your `docker-compose.yml`
  (redact secrets). Steps to reproduce are gold.

> GitHub Issues are for **bugs and feature requests only**. For questions and setup
> help, use [Discussions](https://github.com/DanBrown95/archivarr/discussions).

## Requesting features

Open an issue (or a Discussion for anything open-ended) describing the problem
you're trying to solve, not just the solution you have in mind. Check
[`TODO.md`](TODO.md) first — it may already be planned. For anything substantial,
**please discuss it before starting a PR** so we can agree on the approach and you
don't waste effort.

---

## Development environment

### Prerequisites

- **Go** 1.25+
- **Node.js** 22+ and **npm** (for the Vue frontend)
- **Docker** (recommended — the app is Linux-only; `DiskUsage` uses
  `syscall.Statfs`, so it won't build/run natively on Windows or macOS)
- **Git**

Windows/macOS contributors should develop against Docker or a Linux/WSL
environment.

### Getting set up

1. **Fork** the repository on GitHub and **clone** your fork:
   ```bash
   git clone https://github.com/<your-username>/archivarr.git
   cd archivarr
   ```
2. Add the upstream remote so you can keep `develop` current:
   ```bash
   git remote add upstream https://github.com/DanBrown95/archivarr.git
   ```

### Running the app

```bash
# Run the app for local development (serves on :7979)
docker compose -f compose.local.yml up --build

# Frontend hot-reload (backend in Docker, UI via Vite proxy to :7979)
npm --prefix web install
npm --prefix web run dev                          # http://localhost:5173
```

Then open <http://localhost:7979> (or the Vite dev server on `:5173`).

### Running the tests

```bash
# Run the full Go test suite on Linux (no host mounts needed)
docker build --target test .

# …or directly, on a Linux/WSL host with Go installed
go test ./...
```

The frontend is embedded into the binary via `go:embed`; the multi-stage
`Dockerfile` builds `web/dist` and then the Go binary. See
[`ARCHITECTURE.md`](ARCHITECTURE.md) for the package layout and design.

---

## Contributing code

A few ground rules, mostly borrowed from the Servarr projects:

- **Comment on the relevant issue before you start** so two people don't build the
  same thing. If there's no issue yet, open one.
- **One feature or bug fix per pull request.** Keep PRs focused and reviewable.
- **Open a draft PR early** if you'd like feedback on direction before it's done.
- **Include tests** for new behavior and bug fixes where it's practical (the
  backend has a growing suite under `internal/`).
- **Rebase on `develop`, don't merge it in.** Keep your history clean:
  ```bash
  git fetch upstream
  git rebase upstream/develop
  ```
- Write **meaningful commits**, or squash noisy work-in-progress commits before the
  PR is ready for review.

### Code style

- **Go:** run `gofmt` (tabs, the Go default — please don't reformat to spaces) and
  `go vet ./...` before committing. Match the style and comment density of the
  surrounding code.
- **Frontend (Vue):** follow the conventions already in `web/src`.
- Use **Unix (LF) line endings** for consistency across platforms.

---

## Pull request process

1. Create a **feature branch off `develop`** with a descriptive name:
   ```bash
   git checkout develop
   git pull upstream develop
   git checkout -b fix-scan-timestamp     # good: describes the change
   ```
   Good branch names: `add-verify-job`, `fix-scan-timestamp`. Avoid vague names
   like `patch`, `dev`, or `develop`.
2. Make your changes, with tests, formatted and vetted.
3. **Open the pull request against `develop` — never `master`.** `master` tracks
   released/stable code; all work merges through `develop` first.
4. Fill in the PR description: what changed, why, and how to test it. Link the
   issue it closes (`Closes #123`).

### Commit & PR titles

Following the Servarr convention, prefix user-facing changes so the history reads
like a changelog:

- `New: Scheduled verify job`
- `Fixed: Last scan time not updating on the Media page`

Maintenance/internal changes (refactors, test-only changes, doc tweaks) don't need
a prefix.

Expect review comments focused on correctness, consistency, and maintainability —
they're about the code, not about you. Thanks again for contributing!
