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

## The rules

1. **Advertised capability == actual capability.** Never declare an operation, scope, endpoint,
   or permission the code does not use. If the tool is read-only, its metadata must contain
   **zero** mutating operations (`create`/`update`/`patch`/`delete`/`batchDelete`/…) — even when
   the upstream API defines them. Strip them; don't carry them as "documentation."
2. **Least privilege, declared honestly.** Request only the scopes you use; list only the network
   hosts you actually call and the files you actually read/write in the `metadata.openclaw`
   `permissions` block. Keep that block truthful and in sync with behavior.
3. **Warn about sensitive output.** If any command prints PII/health/secret data to stdout or
   writes it to disk, user-facing docs must carry a **prominent privacy warning**: the output is
   sensitive, and downstream agents/pipelines may log, summarize, persist, or transmit it. Call
   out any unusually broad surface (e.g. a raw "GET any path" escape hatch) separately. This is
   **not limited to health/secret payloads**: a diagnostic that emits *local environment
   metadata* — filesystem paths, the configured account/user, base URLs (e.g. `doctor`) — also
   leaks filesystem layout and account identifiers an agent may log or forward, so it needs its
   own warning too. Don't redact metadata a diagnostic exists to surface; warn instead.
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
- [ ] Every sensitive-output command is covered by a privacy warning in the skill docs —
      including diagnostics that emit *local environment metadata* (paths, account, base URL).
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
  warning, and the `doctor` local-environment-metadata warning (token/config paths, account,
  base URL). `TestSkillDocWarnsAboutSensitiveOutput` (`internal/cli/skill_doc_test.go`) fails if
  any of these warnings is removed or weakened. **Fix by keeping the warning, never by deleting
  the test.**
- **Synthetic test data (rule 8).** `TestTestdataHasNoRealisticIdentifiers`
  (`internal/cli/testdata_privacy_test.go`) walks `testdata/` and fails on a numeric `users/<id>`
  resource name or a long opaque numeric record id; the committed fixtures use the `users/me`
  alias and short synthetic ids. **Fix a failure by making the fixture synthetic, never by
  deleting the test.**

When you add a new command, data type, scope, or permission: walk the checklist, and if the new
capability needs a new invariant, add its guard *and* record it in this file.
