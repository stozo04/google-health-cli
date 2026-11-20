# Releasing google-health-cli

Releases are cut by tagging a commit `vX.Y.Z`. A `v*` tag triggers the **Release**
workflow (`.github/workflows/release.yml`), which runs **GoReleaser**: it cross-compiles
the binary for linux/darwin/windows × amd64/arm64, stamps the version via ldflags,
generates `checksums.txt`, and publishes a GitHub Release with the archives attached.

Versioning is [semver](https://semver.org): bump **major** for a breaking change to the
CLI surface / JSON contract (see `AGENTS.md`), **minor** for new commands or data types,
**patch** for fixes. The version is *your* decision — there is intentionally no auto-tagging.

## Before you release

- The change is merged to `main` and **CI is green** on `main`.
- Locally: `make check` passes (skill mirrors, ClawHub package, tidy, fmt, vet, lint, race tests).
- You've decided the version bump (major/minor/patch).

## How to release

### A. From the GitHub UI (preferred)

**Actions → Release → "Run workflow"** → enter the version (e.g. `v1.2.3`) → Run.

The workflow validates the version, creates and pushes the tag at the current `main`
commit, and runs GoReleaser. (The tag is pushed with the built-in `GITHUB_TOKEN`, which
does not start a second workflow run, so there is no duplicate release.)

### B. From the terminal

```sh
git checkout main && git pull
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

The pushed tag triggers the same Release workflow.

## After it runs

- Confirm the **GitHub Release** appears with six archives + `checksums.txt`:
  `gh release view v1.2.3`
- Confirm a downloaded binary reports the right version: `google-health-cli version`.
- `go install github.com/stozo04/google-health-cli/cmd/google-health-cli@latest` picks up
  the new tag.

Pre-release tags (e.g. `v1.2.3-rc.1`) are auto-marked as prereleases by GoReleaser
(`prerelease: auto` in `.goreleaser.yaml`).

## ClawHub (separate)

The ClawHub skill is **not** tied to releases. It republishes whenever `SKILL.md`,
`OAUTH_SETUP.md`, `docs/MACHINE_CONTRACT.md` or `.clawhubignore` changes on `main`, via `.github/workflows/publish-clawhub.yml`
(needs the `CLAWHUB_TOKEN` repo secret). Bumping the CLI version does not require a ClawHub republish.

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
