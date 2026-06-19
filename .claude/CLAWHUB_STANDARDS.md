# ClawHub Standards

Standards for any skill in this repo that is published to and inspected by **ClawHub**.
This file is **tracked in git** (unlike `CLAUDE.md`, which is local-only) so the rules are
shared across machines, collaborators, and CI. It is loaded into Claude Code via an `@import`
from `CLAUDE.md` — treat it as **required reading before changing skill metadata, scopes,
permissions, docs, or the data-type catalog**.

The governing principle: **what the skill *advertises* must equal what it *does*.** ClawHub
flags the gap between description and behavior; the cheapest way to pass is to never create the
gap, and to back every promise with a guard that fails the build.

## What ClawHub inspects for

- **Description-Behavior Mismatch** — capability metadata (operation catalogs, OAuth scopes,
  `permissions` blocks, tool schemas) advertises something the binary doesn't actually do. In an
  agent setting, that metadata is treated as a capability grant, so a read-only tool that *lists*
  `create`/`update`/`delete` reads as latent write/exfiltration capability.
- **Missing User Warnings** — the skill emits sensitive data (PII, health, financial, secrets,
  tokens) without warning the caller that downstream agents may log, summarize, persist, or
  transmit it.
- **Vulnerability Patterns**
  - *Data Exfiltration*: external transmission to undeclared endpoints, environment-variable
    harvesting, filesystem enumeration beyond what's documented.
  - *MCP Tool Poisoning*: hidden instructions in descriptions, invisible/deceptive Unicode,
    prompt injection smuggled into parameter descriptions or examples.

### Full scan taxonomy (what a ClawHub review checks)

ClawHub runs a broad battery of detectors. Below is the full category set as of this writing,
each with **this repo's posture** — most map to a rule below; a few are inherently **N/A** for a
read-only, self-contained Go CLI, and where there is no dedicated guard yet it says **posture
only**. Keep this list current as the tool grows, and convert a "posture only" line into a real
guard the moment a change makes that category exploitable.

- **Prompt Injection** (instruction override, hidden instructions, exfiltration commands) —
  `SKILL.md`, the catalog, flag help, and examples *describe*; they never instruct an agent to
  act. → rule 4.
- **Data Exfiltration** (external transmission, env-var harvesting, filesystem enumeration) —
  egress is only the declared Google hosts; env reads are the specific `GOOGLE_HEALTH_*` keys via
  `os.LookupEnv` (never an `os.Environ()` sweep); no filesystem walking beyond the declared
  config/token paths. → rules 2, 5.
- **Privilege Escalation** (excessive permissions, sudo/root execution, credential access) — six
  read-only scopes (least privilege); never elevates or runs as root; touches only its own `0600`
  token cache. → rules 1–2; CLI_CONVENTIONS §3.
- **Supply Chain** (unpinned dependencies, external script fetching, obfuscated code) — Go modules
  pin every dependency by version **and** checksum (`go.sum`, enforced by the toolchain on every
  CI build); nothing fetches remote scripts/code at build or run; no obfuscated/minified code.
  → guarded by `TestGoModHasNoReplaceDirectives` (no dependency redirection); never add
  `curl | sh`-style tooling.
- **Excessive Agency** (unrestricted tool access, autonomous decision-making, scope creep) — the
  CLI is a *dumb data collector*: read-only, makes no decisions, does no filtering/merging/
  interpretation (the consumer owns that). Catalog scope creep is blocked by the read-only
  allowlist. → rule 1 + the architecture invariant.
- **Output Handling / Missing User Warnings** (unvalidated output injection, cross-context output,
  unbounded output; sensitive data emitted without a caller warning) — stdout is the API's raw JSON
  bytes, verbatim; the **Privacy / data-minimization / consent** callouts (in both `SKILL.md` and
  `AGENTS.md`) warn that this output crosses into agent/log/pipeline contexts, tell the caller to
  request the narrowest data and obtain owner consent, and flag the OAuth secrets + token as
  sensitive plaintext on disk; counts/hints go to stderr. Callers must treat stdout as untrusted
  data. → rule 3.
- **System Prompt Leakage** (direct, indirect, tool-based) — **N/A**: a CLI binary carries no
  system prompt, and `SKILL.md` hides no instructions to leak.
- **Memory Poisoning** (persistent context injection, context-window stuffing, memory
  manipulation) — **N/A** for the binary (it holds no memory/agent state); the live concern is
  that its *output and descriptions* carry no injection that could poison a consuming agent's
  context. → rules 3–4.
- **Tool Misuse** (parameter abuse, chaining abuse, unsafe defaults) — defaults are safe:
  `api get` is GET-only (no path can write/delete), window flags default to a bounded range,
  secrets default to `0600`. → rule 1; CLI_CONVENTIONS §3.
- **Rogue Agent** (self-modification, session persistence) — the binary never rewrites itself or
  installs background persistence; the only persisted state is the declared token cache. → rule 2;
  CLI_CONVENTIONS §1–2.
- **Trigger Abuse** (overly broad trigger, shadow command, keyword baiting) — the `SKILL.md`
  `name`/`description` describe exactly what the tool does (read Google Health data); no
  keyword-baiting to fire on unrelated prompts, no shadow/undocumented commands. → governing
  principle + rule 4.
- **Behavioral AST** (`exec()`, `eval()`, dynamic import) — no `eval`, no `plugin.Open`/dynamic
  import, no `unsafe`. The **only** process spawn is `auth.OpenBrowser` for the one-time
  interactive login: a hardcoded per-OS launcher (`rundll32`/`open`/`xdg-open`) with the URL as a
  separate argv element and **no shell** (annotated as a G204 false positive). → guarded by
  `TestNoUndeclaredProcessExecutionOrDynamicCode`, which parses the shipped (non-test) source and
  fails if any file outside the `internal/auth/oauth.go` allowlist imports `os/exec`, `plugin`, or
  `unsafe`.
- **Taint Tracking** (direct / variable-mediated taint flow, credential exfiltration chain) — the
  OAuth token flows only into the `Authorization` header of requests to the declared Google host;
  it is never written to stdout, logs, or any other sink. Keep credentials off every output path.
  → rules 3, 5.
- **YARA Signatures** (malware, webshell, cryptominer) — **N/A**: introduce nothing that matches;
  the repo ships a single self-contained client.
- **MCP Least Privilege** (underdeclared capability, wildcard permission, missing permission
  declaration) — the `metadata.openclaw.permissions` block lists the exact network hosts and the
  exact files read/written — no wildcards, nothing omitted. → rule 2.
- **MCP Tool Poisoning** (hidden instructions, Unicode deception, parameter-description injection)
  — no hidden instructions, no invisible/look-alike Unicode, no injection text in any description
  or example. → rule 4.

## The rules

1. **Advertised capability == actual capability.** Never declare an operation, scope, endpoint,
   or permission the code does not use. If the tool is read-only, its metadata must contain
   **zero** mutating operations (`create`/`update`/`patch`/`delete`/`batchDelete`/…) — even when
   the upstream API defines them. Strip them; don't carry them as "documentation."
2. **Least privilege, declared honestly.** Request only the scopes you use; list only the network
   hosts you actually call and the files you actually read/write in the `metadata.openclaw`
   `permissions` block. Keep that block truthful and in sync with behavior.
3. **Warn about sensitive output — and tell the caller how to handle it.** If any command prints
   PII/health/secret data to stdout or writes it to disk, user-facing docs must carry a
   **prominent privacy warning**: the output is sensitive, and downstream agents/pipelines may log,
   summarize, persist, or transmit it. Call out any unusually broad surface (e.g. a raw "GET any
   path" escape hatch) separately. This is **not limited to health/secret payloads**: a diagnostic
   that emits *local environment metadata* — filesystem paths, the configured account/user, base
   URLs (e.g. `doctor`) — also leaks filesystem layout and account identifiers an agent may log or
   forward, so it needs its own warning too. Don't redact metadata a diagnostic exists to surface;
   warn instead. A bare "this is sensitive" line is **not enough** — the warning must also give:
   - **data-minimization guidance** — request the **narrowest** window and only the data types
     actually needed; prefer the typed commands over the broad `api get` escape hatch; don't
     persist/forward output beyond the task;
   - **operator-consent expectations** — run only against an account whose owner has knowingly
     consented; make clear the one-time `auth login` is the owner consenting to *read-only access
     for the tool*, **not** consent for a downstream agent to collect/retain/transmit more broadly;
   - **credential-protection guidance** — the OAuth `client_id`/`client_secret` and the cached
     token are **sensitive secrets stored on disk in plaintext**; say they're written `0600` and
     gitignored, and tell the caller to keep them out of commits, shared dirs, backups, and logs
     (and to rotate / `auth logout` on a suspected leak). Encouraging plaintext secret storage
     *without* this warning is itself a ClawHub "Missing User Warnings" finding.

   These warnings must appear in **both** the human `SKILL.md` **and** the `AGENTS.md` machine
   contract (a ClawHub reviewer reads `AGENTS.md` as "the contract"), and each is pinned by a guard
   (see "How this repo enforces the rules").
4. **No hidden or deceptive content.** No instructions embedded in parameter/field descriptions,
   no invisible or look-alike Unicode, no prompt-injection text in examples or sample data.
   Descriptions describe; they never instruct the agent to act.
5. **No undeclared egress or harvesting.** Network calls go only to declared endpoints. Don't
   read environment variables you don't need, don't enumerate the filesystem, don't phone home.
6. **Enforce, don't just document.** Every invariant above must be backed by a guard that
   **fails the build** — an init-time check, a unit test, or a lint rule — not prose alone. A
   standards doc prevents *writing* the violation; only a guard prevents *shipping* it. When you
   document a rule here, add (or point to) its guard.
7. **Documentation paths must be community-friendly — never machine-specific.** Any path in
   tracked docs, comments, examples, or commit messages must be **generic and portable**: no
   absolute paths, no home directories, no usernames, no machine-local install locations (e.g.
   `C:\Users\<name>\...`, `/home/<name>/...`, `%AppData%\...`). Use repo-relative paths,
   placeholders (`<path-to-go>`, `$(go env GOPATH)`, `~`), or environment variables instead.
   Leaking a local layout is noise to contributors and a minor info-disclosure smell; treat the
   docs as if a stranger will read them, because once published they will.
8. **Test data must be unmistakably synthetic.** Committed fixtures, golden files, and doc
   examples must never contain real or real-looking individualized records — no persistent
   **numeric user ids** (use the `users/me` alias), no long opaque production record ids (use
   short placeholders), no real account/device identifiers. ClawHub flags real-looking health
   records + a persistent identifier as a re-identification risk even when the data is fabricated,
   because once the repo is shared it can't tell. Keep the *shape* realistic for the test; keep
   the *identity* fake.
9. **Self-review against this file immediately before opening a PR.** Right before you create or
   update a PR, **re-read this file and walk the pre-publish checklist against the actual diff** —
   not from memory, and not "later." Treat it as a required gate: if the diff touches skill
   metadata, scopes, permissions, docs, the data-type catalog, fixtures, or any command's output,
   confirm each rule still holds and each new capability ships with its guard in the **same** PR.
   The cheapest ClawHub finding to fix is the one you catch before the PR exists.

- [ ] Metadata lists only operations/scopes/permissions the code actually exercises.
- [ ] No mutating op appears anywhere a read-only tool's metadata is generated or embedded.
- [ ] Every sensitive-output command is covered by a privacy warning in **both** `SKILL.md` and
      `AGENTS.md` — including diagnostics that emit *local environment metadata* (paths, account,
      base URL).
- [ ] The privacy warning also gives data-minimization guidance, operator-consent expectations,
      and a credential-protection warning (OAuth secrets + token are sensitive plaintext on disk).
- [ ] No hidden instructions, deceptive Unicode, or injection text in descriptions/examples.
- [ ] Network egress and file access match the declared `permissions` block exactly.
- [ ] No absolute paths, home dirs, usernames, or machine-local locations in tracked docs/comments/examples.
- [ ] Fixtures, goldens, and examples are synthetic — no numeric user ids (`users/me`), no
      long opaque record ids, no real account/device identifiers.
- [ ] Each of the above is enforced by a test or startup guard, not just documented.
- [ ] `make check` (or the repo's equivalent) is green.
- [ ] **Immediately before opening the PR**, re-read this file and walk this checklist against
      the actual diff (rule 9).

## How **this repo** enforces the rules

Concrete guards already in place — keep them, and add to them when you add capability:

- **Read-only catalog (rules 1–2).** `internal/api/datatypes.json` may advertise only the
  `readOnlyOps` allowlist (`list`, `get`, `reconcile`, `rollup`, `dailyRollUp`). `init()` in
  `internal/api/datatypes.go` **panics at startup** on any op outside the allowlist, and
  `TestCatalogIsReadOnly` (`internal/api/datatypes_test.go`) asserts no mutating op and no
  unknown op. Verified to fail when a write op is reintroduced.
- **Sensitive-output warnings (rule 3).** `SKILL.md` carries the Privacy callout (stdout is
  health PII that may be logged/transmitted/persisted), the `api get` sensitive-endpoint
  warning, the `doctor` local-environment-metadata warning (token/config paths, account, base
  URL), the data-minimization & operator-consent callout, and the "Protect your credentials"
  plaintext-secrets warning. `AGENTS.md` (the machine contract) carries the same set in its
  "Privacy, data minimization & consent" section. `TestSkillDocWarnsAboutSensitiveOutput` and
  `TestAgentsDocWarnsAboutPrivacyAndConsent` (`internal/cli/skill_doc_test.go`) fail if any of
  these warnings is removed or weakened in either doc. **Fix by keeping the warning, never by
  deleting the test.**
- **Synthetic test data (rule 8).** `TestTestdataHasNoRealisticIdentifiers`
  (`internal/cli/testdata_privacy_test.go`) walks `testdata/` and fails on a numeric `users/<id>`
  resource name or a long opaque numeric record id; the committed fixtures use the `users/me`
  alias and short synthetic ids. **Fix a failure by making the fixture synthetic, never by
  deleting the test.**
- **No undeclared process execution / dynamic code (Behavioral AST, Excessive Agency).**
  `TestNoUndeclaredProcessExecutionOrDynamicCode` (`internal/cli/source_guards_test.go`) parses
  every shipped (non-test) `.go` file and fails if any file outside the `internal/auth/oauth.go`
  allowlist imports `os/exec`, `plugin`, or `unsafe`. Its matcher is unit-tested
  (`TestGuardedImportViolationLogic`) and verified to fail when an exec import is reintroduced.
- **No dependency redirection (Supply Chain).** `TestGoModHasNoReplaceDirectives`
  (`internal/cli/source_guards_test.go`) fails if `go.mod` carries a `replace` directive; the
  detector is unit-tested (`TestReplaceDirectiveDetection`). `go.sum` checksums are enforced by the
  toolchain on every build.

When you add a new command, data type, scope, or permission: walk the checklist, and if the new
capability needs a new invariant, add its guard *and* record it in this file.
