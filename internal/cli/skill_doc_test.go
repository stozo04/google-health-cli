package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test file until it finds the module's go.mod, so
// the docs tests work regardless of the test's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine caller path")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found walking up from %s", file)
		}
		dir = parent
	}
}

// TestSkillDocWarnsAboutSensitiveOutput is the immutable guard for the "missing
// user warnings" findings. SKILL.md must carry (1) a prominent privacy warning
// that the JSON on stdout is sensitive health data that downstream agents may
// log/transmit/persist, (2) a warning that the `api get` escape hatch reaches
// sensitive profile/settings endpoints, (3) a warning that `doctor` prints
// local environment metadata (token/config paths, account, base URL) an agent may
// log or forward, (4) data-minimization guidance and operator-consent
// expectations, and (5) a warning that the OAuth credentials and cached token are
// sensitive plaintext secrets to protect. If any warning is removed or weakened,
// this test fails — fix by keeping the warning, never by deleting the test.
func TestSkillDocWarnsAboutSensitiveOutput(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	skill := string(raw)

	required := []struct {
		why    string
		marker string
	}{
		{"a privacy header for stdout data", "Privacy"},
		{"names the sensitive sink behaviors", "logged, summarized, persisted, or transmitted"},
		{"flags the output as PII", "PII"},
		{"warns the api get escape hatch reaches sensitive endpoints", "Sensitive endpoints"},
		{"calls out the profile/settings reach of api get", "users/me/profile"},
		{"warns doctor emits local environment metadata", "local environment metadata"},
		{"names doctor's path/account output", "token-cache and config file paths"},
		{"gives data-minimization guidance", "Data minimization"},
		{"sets operator-consent expectations", "knowingly consented"},
		{"warns the credentials are secrets to protect", "Protect your credentials"},
		{"notes the credentials are stored in plaintext", "sensitive secrets stored on disk in plaintext"},
		{"documents the runtime stderr privacy notice", "privacy notice to stderr"},
		{"documents api get is limited to the v4 surface", "constrained to the read-only v4 surface"},
	}
	for _, r := range required {
		if !strings.Contains(skill, r.marker) {
			t.Errorf("SKILL.md missing required warning (%s): %q not found", r.why, r.marker)
		}
	}
}

// TestAgentsDocWarnsAboutPrivacyAndConsent is the immutable guard mirroring the
// SKILL.md warnings into AGENTS.md — the machine contract a ClawHub reviewer reads
// as "the contract." It must carry a prominent privacy/data-minimization/consent
// section: the sensitive-stdout warning, data-minimization guidance, operator
// consent expectations, the credentials-are-plaintext-secrets warning, and the
// api get profile/settings reach. Fix a failure by restoring the warning, never by
// deleting the test.
func TestAgentsDocWarnsAboutPrivacyAndConsent(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "docs", "MACHINE_CONTRACT.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	agents := string(raw)

	required := []struct {
		why    string
		marker string
	}{
		{"a prominent privacy/minimization/consent section", "Privacy, data minimization & consent"},
		{"names the sensitive sink behaviors", "logged, summarized, persisted, or transmitted"},
		{"gives data-minimization guidance", "Data minimization"},
		{"sets operator-consent expectations", "Operator consent"},
		{"reinforces that consent is read-only, not downstream collection", "knowingly consented"},
		{"warns the credentials are sensitive plaintext secrets", "Credentials are sensitive secrets"},
		{"flags the api get profile/settings reach", "users/me/profile"},
		{"documents the runtime stderr privacy notice", "privacy notice to stderr at run time"},
		{"documents api get is limited to the v4 surface", "constrained to the read-only v4 surface"},
	}
	for _, r := range required {
		if !strings.Contains(agents, r.marker) {
			t.Errorf("AGENTS.md missing required warning (%s): %q not found", r.why, r.marker)
		}
	}
}

func TestOAuthSetupDocWarnsAboutCredentialsAndUnverifiedApp(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "OAUTH_SETUP.md"))
	if err != nil {
		t.Fatalf("read OAUTH_SETUP.md: %v", err)
	}
	setup := string(raw)

	required := []struct {
		why    string
		marker string
	}{
		{"warns the credentials are secrets to protect", "Protect your credentials"},
		{"notes the credentials are stored in plaintext", "sensitive secrets stored on disk in plaintext"},
		{"tells the user to keep secrets out of commits", "Never commit them"},
		{"gives leak remediation", "auth logout"},
		{"limits the unverified-app bypass to the user's own client", "Only click through this warning for an OAuth client you created yourself"},
	}
	for _, r := range required {
		if !strings.Contains(setup, r.marker) {
			t.Errorf("OAUTH_SETUP.md missing required warning (%s): %q not found", r.why, r.marker)
		}
	}
}

func TestSkillDocDeclaresToolScopeAndConfigBoundary(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	skill := string(raw)
	frontmatter, _, ok := strings.Cut(strings.TrimPrefix(skill, "---\n"), "\n---\n")
	if !ok {
		t.Fatal("SKILL.md has no frontmatter block")
	}
	for _, declared := range []string{
		"\npermissions:\n",
		`"network: HTTPS to accounts.google.com, oauth2.googleapis.com and health.googleapis.com only, plus a one-time 127.0.0.1 listener for the auth login callback"`,
		`"file_read: config.json and the OAuth token cache"`,
		`"file_write: the OAuth token cache only; it is deleted on auth logout"`,
		`"env: the five GOOGLE_HEALTH_* variables listed under envVars"`,
		`"shell: launches the system browser once during auth login`,
	} {
		if !strings.Contains(frontmatter, declared) {
			t.Errorf("SKILL.md frontmatter must declare its tool scope: %q not found", declared)
		}
	}
	for _, marker := range []string{
		"Config can't redirect your token or widen access",
		"only `https://health.googleapis.com`",
		"no parseable start time",
	} {
		if !strings.Contains(skill, marker) {
			t.Errorf("SKILL.md missing %q", marker)
		}
	}
}
