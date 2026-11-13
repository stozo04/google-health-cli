#!/usr/bin/env node
import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";
import { join, resolve } from "node:path";

const SHIPPABLE = [
  /^SKILL\.md$/,
  /^OAUTH_SETUP\.md$/,
  /^README\.md$/,
  /^LICENSE$/,
  /^config\.example\.json$/,
  /^docs\/MACHINE_CONTRACT\.md$/,
  /^go\.(mod|sum)$/,
  /^(cmd|internal)\/.+(?<!_test)\.go$/,
  /^internal\/api\/datatypes\.json$/,
];

const REQUIRED = [
  "SKILL.md",
  "OAUTH_SETUP.md",
  "docs/MACHINE_CONTRACT.md",
  "go.mod",
  "go.sum",
  "cmd/google-health-cli/main.go",
  "internal/api/datatypes.json",
];

const CREDENTIAL_ACCESS_PROSE = /access\s+(?:the\s+)?(?:credentials?|secrets?|tokens?)/i;

function usage() {
  console.error("usage: check-clawhub-package.mjs <unpacked clawhub package dir> [repo root]");
  process.exit(2);
}

const [clawhubDir, repoArg] = process.argv.slice(2);
if (!clawhubDir) usage();
const repoRoot = resolve(repoArg ?? ".");

if (!CREDENTIAL_ACCESS_PROSE.test("the on-disk access token")) {
  console.error("credential-access matcher no longer fires on the phrase SkillSpector flags");
  process.exit(2);
}

const walkerURL = pathToFileURL(join(resolve(clawhubDir), "dist", "publishFiles.js")).href;
const { forEachPublishFile } = await import(walkerURL);

const shipped = [];
await forEachPublishFile(repoRoot, { includeDotfiles: false, respectGitignore: true }, (_abs, rel) => {
  shipped.push(rel);
});
shipped.sort();

const unexpected = shipped.filter((f) => !SHIPPABLE.some((re) => re.test(f)));
const missing = REQUIRED.filter((f) => !shipped.includes(f));

const credentialProse = [];
for (const rel of shipped) {
  const lines = (await readFile(join(repoRoot, rel), "utf8")).split("\n");
  lines.forEach((line, i) => {
    if (CREDENTIAL_ACCESS_PROSE.test(line)) credentialProse.push(`${rel}:${i + 1}: ${line.trim()}`);
  });
}

console.log(`clawhub would publish ${shipped.length} files:`);
for (const f of shipped) console.log(`  ${f}`);

if (unexpected.length || missing.length || credentialProse.length) {
  for (const f of unexpected) console.error(`not shippable (add it to .clawhubignore or SHIPPABLE): ${f}`);
  for (const f of missing) console.error(`required file missing from the package: ${f}`);
  for (const hit of credentialProse) {
    console.error(`SkillSpector reports this as High "Credential Access" (PE3 prose rule); say "OAuth token" instead: ${hit}`);
  }
  process.exit(1);
}
