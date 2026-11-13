package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var clawhubExcludedFromSkillPackage = []string{
	"*_test.go",
	"testdata/",
	"scripts/",
	"AGENTS.md",
	"CLAUDE.md",
	"Engineering-Prompt.md",
	"RELEASING.md",
	"docs/OPERATING_INSTRUCTIONS.md",
	"docs/PROJECT_INSTRUCTIONS.md",
	"docs/project-agent-audit.md",
}

var clawhubShippedSkillFiles = []string{
	"SKILL.md",
	"OAUTH_SETUP.md",
	"README.md",
	"docs/MACHINE_CONTRACT.md",
}

func clawhubIgnorePatterns(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".clawhubignore"))
	if err != nil {
		t.Fatalf("read .clawhubignore: %v", err)
	}
	patterns := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if pattern := strings.TrimSpace(line); pattern != "" && !strings.HasPrefix(pattern, "#") {
			patterns[pattern] = true
		}
	}
	return patterns
}

func TestClawhubPackageExcludesRepoTooling(t *testing.T) {
	patterns := clawhubIgnorePatterns(t)
	for _, want := range clawhubExcludedFromSkillPackage {
		if !patterns[want] {
			t.Errorf(".clawhubignore must exclude %q so ClawHub scans only the skill, not repo tooling", want)
		}
	}
}

func TestClawhubPackageKeepsSkillDocs(t *testing.T) {
	patterns := clawhubIgnorePatterns(t)
	for _, shipped := range clawhubShippedSkillFiles {
		if patterns[shipped] {
			t.Errorf(".clawhubignore must not exclude %q: it is part of the published skill", shipped)
		}
		if _, err := os.Stat(filepath.Join(repoRoot(t), shipped)); err != nil {
			t.Errorf("published skill file %q is missing: %v", shipped, err)
		}
	}
}
