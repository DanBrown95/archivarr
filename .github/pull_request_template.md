<!--
Thanks for contributing to Archivarr! Please read CONTRIBUTING.md first.
Key points:
  • PRs target `develop`, never `master`.
  • Rebase on `develop` — don't merge it in.
  • One feature or bug fix per PR.
  • Prefix user-facing changes in the title: "New: …" or "Fixed: …".
-->

## Description

<!-- What does this PR do, and why? -->

## Related issue

<!-- Link the issue this addresses, e.g. "Closes #123". Open one first for substantial changes. -->
Closes #

## Type of change

- [ ] Bug fix (`Fixed: …`)
- [ ] New feature (`New: …`)
- [ ] Maintenance / refactor / docs (no prefix needed)

## How to test

<!-- Steps for a reviewer to verify the change. -->

## Checklist

- [ ] This PR targets the **`develop`** branch (not `master`).
- [ ] Branch is **rebased on** the latest `develop` (not merged).
- [ ] Code is formatted (`gofmt`) and vetted (`go vet ./...`); frontend follows `web/src` conventions.
- [ ] Tests added/updated where practical, and `go test ./...` passes.
- [ ] One focused feature or bug fix (not a grab-bag of unrelated changes).
