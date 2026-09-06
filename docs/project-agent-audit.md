# Project agent audit, 2026-09-06

Scope: google-health-cli on remote main, isolated from the existing feature checkout. No previous task PR or prepared audit branch existed. Read AGENTS.md, CLAUDE.md, both .claude convention/standards documents, root SKILL.md, OAuth setup, README, releasing instructions, Makefile, CI and publishing configuration, and affected documentation guards. Dependencies, build output, archived copies, and nested worktrees were excluded.

Sources: [GPT-6 Astra guide](https://developers.openai.com/api/docs/guides/latest-model?model=gpt-6-astra) and [OpenLoop PR 176](https://github.com/stozo04/OpenLoop/pull/176).

Reconciled the unique machine contract and developer guidance into docs/MACHINE_CONTRACT.md and docs/PROJECT_INSTRUCTIONS.md without dropping privacy warnings, JSON shapes, exit codes, architecture, or release gates. Root pointers are identical. Cursor has an always-applied rule. The local CLI conventions remain byte-identical to their starting version; no other repository is required to read them. Native shared standards remain in .claude with shared discovery. Updated the existing warning test's document path, preserving every assertion; release archives include the documents referenced by the pointers.

The published root usage package was the only existing skill source. Copied its SKILL.md and OAuth setup support file to all three skill trees; each copy matches the root bytes. The bounded-query guidance now prevents an automatic --all fallback from widening authorized health-data access. Retained all consent, plaintext-credential, read-only scope, stderr privacy, and release/publish boundaries. Metadata permissions remain unchanged. No provider settings or hooks existed to migrate.

Reused OpenLoop's synchronization checker and 16 regressions. Integrated make check-agents into make check and existing CI. Python is a development-check dependency only; the shipped Go binary still has no Python runtime dependency.

Validation: synchronization reports 2 files x 3; all 16 checker regressions pass, including both slash formats and preservation of intended source bytes during repair. Root pointers and all root-to-package copies independently compared. Existing privacy assertions are retained at the relocated contract. Final diff inspected with git diff --check.

Skipped: live OAuth, health-data reads, ClawHub publication/inspection, release archives, and interactive agent behavior evaluation. These need external actions outside this documentation audit. No merge, release, or deployment performed.

Required make check passed: tidy, formatting, vet, lint with 0 issues, and race tests in all five test-bearing packages. Two packages have no test files.
