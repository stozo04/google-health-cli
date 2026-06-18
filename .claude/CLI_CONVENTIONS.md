# Shared CLI Conventions — google-health-cli & speediance-cli

**Locked 2026-06-18** (converged between the GH and SPD agents). This file is
committed **byte-identical** to both repos and `@import`-ed from each `CLAUDE.md`,
the same way `.claude/CLAWHUB_STANDARDS.md` already is. Propose changes through the
shared agent process so both copies stay in sync.

These are two self-contained, read-only, agent-first CLIs (one Go binary, no
runtime deps) that share a config/auth/credential layer. This file captures the
invariants that layer must uphold in **both** repos.

**Governing principle:** parity is about **shared patterns and safety
invariants, not identical strings.** Env vars, JSON keys, and app-dir names
legitimately differ per app — each invariant below states the *pattern* and lists
**per-repo values** in a table, so neither repo has to fork this file. Every
invariant is backed by a **guard that fails the build** (test or startup check),
never prose alone.

| | google-health-cli (GH) | speediance-cli (SPD) |
|---|---|---|
| binary | `google-health-cli` | `speediance-cli` |
| app-dir constant | `appDirName = "google-health-cli"` | `appUserSubdir = "speediance"` |
| credential env prefix | `GOOGLE_HEALTH_*` | `SPEEDIANCE_*` |
| reads `.env`? | no (os.LookupEnv only) | yes (allowlisted keys only) |

---

## 0. Cornerstone — Advertised capability == actual capability

Every capability the tool *advertises* must equal what the binary *does*: config
keys, env vars, OAuth scopes, operation catalogs, `SKILL.md` permission blocks,
tool schemas. A gap is treated as a latent capability grant (a read-only tool
that lists a write op reads as write capability; an advertised config key the
loader never parses is a silent lie). This is the load-bearing invariant; the
others are specializations of it.

**Guard:** each advertisement is pinned by a **negative-assertion** regression
test where automatable; otherwise by an **enforced publish scan / checklist named
in the table** and tracked toward a test.

| | GH | SPD |
|---|---|---|
| guards | `TestCatalogIsReadOnly` (no mutating op in catalog), `TestSkillDocWarnsAboutSensitiveOutput` | `TestTokenCacheConfigKeyHonored` (advertised `token_cache_path` now actually wired — a literal advertised==actual test), `TestDotEnvDoesNotInjectForeignEnv`; SKILL-perms==code via ClawHub publish scan + manual pre-publish checklist (automated test = SPD adoption-PR TODO) |

## 1. Per-user state dir — never a CWD secret

Secrets (token cache, any written credential) live under
`os.UserConfigDir()/<app-dir>/`, **never** a CWD-relative path. A CLI's working
directory is attacker-/agent-influenceable and frequently a *different* repo, so a
CWD secret leaks (and gets committed). Fall back to a relative path only as an
absolute last resort (UserConfigDir unavailable).

**Guard:** a negative test asserting the default secret path does **not** resolve
inside the CWD.

| | GH | SPD |
|---|---|---|
| token file | `<UserConfigDir>/google-health-cli/token.json` | `<UserConfigDir>/speediance/token.json` |
| guard | `TestTokenCacheDefaultNotInWorkingDir` *(to add in adoption PR)* | `TestTokenCacheDefaultNotInWorkingDir` |

## 2. One app-dir constant, for both token and config

A **single** constant names the per-user app dir, used by *both* the token-cache
default and the config-discovery fallback (so state isn't split across two
folders). **Value = the repo's already-established app-dir** — the binary name for
a greenfield repo; the dir that already shipped if one exists. (This is decision
D2 — "don't rename existing things for cosmetic parity" — applied to a directory.)

| | GH | SPD |
|---|---|---|
| value | `google-health-cli` (= binary, greenfield) | `speediance` (pre-existing `<UserConfigDir>/speediance/config.json` fallback) |

## 3. Secret file permissions

Secrets are written `0600`; their parent dir is created `0700`; `Chmod` is
best-effort and a failure is ignored (Windows treats the Unix bits as advisory).

| | GH | SPD |
|---|---|---|
| writer | `auth.SaveToken` | `auth.Save` |

## 4. Discovery order: documented, in sync, and self-describing

Discovery order is **documented and kept in sync with behavior**. Recording the
consulted locations programmatically — so diagnostics name the real search path
and can't drift — is the **recommended** mechanism, not a hard requirement: GH
does this via `Config.SearchedPaths`; SPD keeps the order documented in
`config.go` for its 3-step discovery and adopts a record if the surface grows.

| | GH | SPD |
|---|---|---|
| record | `Config.SearchedPaths` | documented order in `config.go`; no programmatic record (3-step discovery) |

## 5. Fail fast with an actionable error — never leak a raw library error

When a precondition is missing (no credentials, no config), fail **before** the
network/refresh call with a message that names **where it looked and the fix**.
Never surface a raw library string (e.g. oauth2 `"Could not determine client
ID"`); translate it into a cause the caller can act on.

| | GH | SPD |
|---|---|---|
| surface | `missingCredentialsError` (exit 64), names `SearchedPaths` | `RequireCredentials` — names env vars + resolved config path (enrich to full search order = SPD adoption-PR TODO) |

## 6. A machine-checkable config + credential state that exits non-zero when broken

There must be a programmatic way to report config + credential health that
**exits non-zero when broken** — *capability, not command.* A dedicated `doctor`
is one way; `config show --json` + an auth command that exits non-zero is another.
Neither form is mandated; the exit-non-zero-when-broken property is.

| | GH | SPD |
|---|---|---|
| surface | `doctor` → `configFound`/`clientIdLoaded`, exit 78 (config) / 2 (auth) | `config show --json` + `login` exit 2 (deliberately **no** `doctor`; see SPD `CLAUDE.md` scope note) |

## 7. Migration on a default-location change — conservative, one-shot

When you change a **default** secret location, relocate the old file forward
exactly once, conservatively:
- **default-path-only** — never scavenge an explicit `--flag`/env override;
- **never clobber** a token already at the destination;
- **no-op** on a missing/corrupt legacy file;
- **best-effort** — a failure just means the next call logs in fresh;
- remove the legacy file **only after** the new copy is safely written.

So no user is forced to re-auth, and the credential actually leaves the old spot.

| | GH | SPD |
|---|---|---|
| status | N/A — token has always been per-user; no default has moved | `auth.MigrateLegacy` (`.token.json` → per-user), `TestMigrateLegacyMovesToken` |

## 8. Portable paths in tracked docs

No absolute paths, home dirs, usernames, or machine-local locations in tracked
docs, comments, examples, or commit messages. Use repo-relative paths,
placeholders, or env vars. (Already each repo's `CLAWHUB_STANDARDS.md §7`.)

## 9. `.env` must never mutate the process environment

If a repo reads `.env`, parse it to a **map** and apply only your own allowlisted
keys — never a loader that `os.Setenv`s every key in the file. A hostile `.env` in
an attacker-influenceable CWD could otherwise inject `PATH`/`LD_PRELOAD`/proxy
vars (a privilege-escalation vector).

| | GH | SPD |
|---|---|---|
| status | N/A — reads no `.env` (`os.LookupEnv` only); adopt this if it ever does | `godotenv.Read()` + allowlist, `TestDotEnvDoesNotInjectForeignEnv` |
