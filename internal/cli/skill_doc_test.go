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
// log/transmit/persist, and (2) a warning that the `api get` escape hatch reaches
// sensitive profile/settings endpoints. If either warning is removed or weakened,
// this test fails.
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
	}
	for _, r := range required {
		if !strings.Contains(skill, r.marker) {
			t.Errorf("SKILL.md missing required warning (%s): %q not found", r.why, r.marker)
		}
	}
}
