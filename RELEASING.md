# Releasing google-health-cli

A release is cut by merging the open release PR (`chore(main): release X.Y.Z`). Merging code
updates `main`; merging the release PR is the deliberate "publish" button.

[release-please](https://github.com/googleapis/release-please-action)
(`.github/workflows/release-please.yml`) runs on every push to `main` and keeps one release PR
open, updating its version and notes as feature and fix PRs land. It only changes
`.github/.release-please-manifest.json`. Merging it tags `vX.Y.Z`, creates the GitHub Release,
and calls the **Release** workflow (`.github/workflows/release.yml`) in the same run. That runs
**GoReleaser**: it cross-compiles the binary for linux/darwin/windows × amd64/arm64, stamps the
version via ldflags, generates `checksums.txt`, attaches the archives and replaces the release
notes with the grouped changelog (`release.mode: replace`).

Versioning is [semver](https://semver.org) and comes from the commits since the last release:
`fix:` bumps **patch**, `feat:` bumps **minor**, `feat!:` or a `BREAKING CHANGE:` footer bumps
**major** (a breaking change to the CLI surface / JSON contract, see `AGENTS.md`). Commits like
`ci:`, `docs:`, `test:` and `chore:` alone don't open a release PR. To force a version, add a
commit to `main` with a `Release-As: X.Y.Z` footer.

The current version lives in `.github/.release-please-manifest.json`; config is in
`.github/release-please-config.json` (under `.github/` so ClawHub never ships them). There is
no `CHANGELOG.md` (`skip-changelog`).

## Before you release

- The changes you want are merged to `main` and **CI is green** on `main`.
- The release PR proposes the version you expect. If not, fix the commit types or add a
  `Release-As:` commit.

## How to release

```sh
gh pr list --label "autorelease: pending"   # the open release PR
gh pr merge <number> --squash --admin       # or "Merge without waiting for requirements" in the UI
gh run watch                                # Release Please → GoReleaser
gh release view vX.Y.Z                      # verify the published release
```

The release PR shows its required checks as **Expected** and needs an admin merge. GitHub starts
no workflows for a PR opened with `GITHUB_TOKEN`, so CI never runs on it; it only bumps the
manifest, and the code it releases already passed CI on `main`.

A tag created with the workflow's own `GITHUB_TOKEN` doesn't start other workflows, which is why
the Release Please workflow calls `release.yml` directly (`workflow_call`) instead of relying on
the tag push.

### Manual fallback

Pushing a tag, or **Actions → Release → "Run workflow"** with a version, still runs
`release.yml` as before:

```sh
git checkout main && git pull
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

If you do this, also update `.github/.release-please-manifest.json` to the same version in a PR,
or release-please will propose a version based on its last release.

## After it runs

- Confirm the **GitHub Release** appears with six archives + `checksums.txt`:
  `gh release view v1.2.3`
- Confirm a downloaded binary reports the right version: `google-health-cli version`.
- `go install github.com/stozo04/google-health-cli/cmd/google-health-cli@latest` picks up
  the new tag.

Pre-release tags (e.g. `v1.2.3-rc.1`) are auto-marked as prereleases by GoReleaser
(`prerelease: auto` in `.goreleaser.yaml`).

## Prerequisites (one-time)

Repo setting **Settings → Actions → General → Workflow permissions → "Allow GitHub Actions to
create and approve pull requests"** must be on, or release-please can't open the release PR.

## ClawHub (separate)

The ClawHub skill is **not** tied to releases. It republishes whenever a file the package ships (or
`.clawhubignore` or `.gitignore`, which decide what ships) changes on `main`, via `.github/workflows/publish-clawhub.yml` (needs the `CLAWHUB_TOKEN`
repo secret). `make check-clawhub` fails if a shipped file is missing from that workflow's `paths`.
Bumping the CLI version does not require a ClawHub republish.

ClawHub hides a new version until its security scans finish (about 10 minutes), and the CLI picks the
next version from the latest public one. When two publishes land inside that window, the second gets
`Version X already exists`. `scripts/publish-clawhub.sh`, which both ClawHub workflows call, retries
as the next patch version (up to five attempts) and fails on any other error. Every publish to the live
listing (a merge to `main` or a `target=live` branch run) waits in one `clawhub-publish-live` queue
(`queue: max`, first in, first out) and none cancels or replaces another, so two can never publish
`google-health-cli` at once and a queued main publish is never dropped.

To scan a branch without merging, run **Deploy to ClawHub (from branch)** with `target=scan-copy`. It
publishes to `google-health-cli-scan-<run id>-<attempt>`, waits for the scan, puts the verdict in the run summary
and the full report in the `clawhub-scan-report` artifact, then deletes the copy (it keeps the copy if the report could not be saved). The weekly **Sweep
ClawHub scan copies** workflow deletes any copy a finished run left behind. Both use
`scripts/clawhub-scan-copies.sh`, which refuses to delete anything outside the `google-health-cli-scan-`
prefix. `make test-scripts` tests both scripts against a fake `clawhub`.
