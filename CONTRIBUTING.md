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
3. **Open the pull request against `develop` — never `master` or a release
   branch.** `develop` is the integration/nightly branch; `master` only ever holds
   tagged, released code. See [Maintainer: release process](#maintainer-release-process)
   for how `develop` becomes a release.
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

---

## Maintainer: release process

This section is for maintainers. **Contributors don't need to do any of this** —
just target your PR at `develop` (above).

Archivarr uses a [Git Flow](https://nvie.com/posts/a-successful-git-branching-model/)-style
branching model with two permanent branches and two kinds of temporary branch.

### Branches

| Branch | Lifetime | Purpose |
| --- | --- | --- |
| `develop` | permanent | Integration branch. **All PRs merge here.** Nightly / test images build from it. |
| `master` | permanent | Stable. Only ever receives a finished release/hotfix merge; **every commit is a tagged release**, and stable images build from it. |
| `release/x.y.z` | temporary | Cut from `develop` to stabilize a release. **Feature-frozen** — only bug fixes. |
| `hotfix/x.y.z` | temporary | Cut from `master` to patch a critical bug in an already-shipped release. |

This gives two update channels: **`develop` = nightly/beta**, **`master` = stable**.

### Cutting a release

1. Branch a release off `develop` when the feature set is ready to stabilize:
   ```bash
   git checkout develop && git pull
   git checkout -b release/0.2.0
   git push -u origin release/0.2.0
   ```
   `develop` immediately reopens for the next version — new PRs keep merging there
   while `release/0.2.0` is stabilized.
2. **Only bug fixes go on the release branch** (version bumps, doc/changelog
   touch-ups, and fixes for issues found while testing). No new features.
3. When it's solid, optionally tag a release candidate to soak-test the image
   first (`v0.2.0-rc.1`), then finish the release:
   ```bash
   # Promote to stable
   git checkout master && git merge --no-ff release/0.2.0
   git tag -a v0.2.0 -m "v0.2.0"
   git push origin master --tags        # CI builds archivarr:0.2.0 + :latest

   # Merge the stabilization fixes BACK into develop (see rule below)
   git checkout develop && git merge --no-ff release/0.2.0
   git push origin develop

   # Done with the release branch
   git branch -d release/0.2.0 && git push origin --delete release/0.2.0
   ```

### Hotfixing a shipped release

For a critical bug in the current stable release when `develop` isn't ready to ship:

```bash
git checkout -b hotfix/0.2.1 master
# …fix the bug…
git checkout master && git merge --no-ff hotfix/0.2.1
git tag -a v0.2.1 -m "v0.2.1"
git push origin master --tags

git checkout develop && git merge --no-ff hotfix/0.2.1   # back-merge (see rule)
git push origin develop
git branch -d hotfix/0.2.1 && git push origin --delete hotfix/0.2.1
```

### The one rule that keeps bugs out

**Every fix made on a `release/*` or `hotfix/*` branch MUST be merged back into
`develop`.** That's why each flow above ends with a merge into `develop`. Skip it
and the bug reappears in the next release, because `develop` never received the
fix. Because fixes are authored on the release/hotfix branch and flow back via a
merge, there's **no cherry-picking** in the normal workflow.

### Versioning

Tags follow [Semantic Versioning](https://semver.org/) (`vMAJOR.MINOR.PATCH`).
Until the first stable tag, expect `0.x` releases where minor bumps may include
breaking changes.

### GitHub settings to enforce this

- Set **`develop` as the default branch** (Settings → General → Default branch) so
  new PRs and clones target it automatically.
- Protect **`master`** and **`develop`** (Settings → Branches): require pull
  requests, disallow direct pushes, and require status checks once CI exists (see
  [`TODO.md`](TODO.md)).
