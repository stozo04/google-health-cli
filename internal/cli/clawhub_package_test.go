package cli

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
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

var skillSpectorCredentialAccessProse = regexp.MustCompile(`(?i)access\s+(?:the\s+)?(?:credentials?|secrets?|tokens?)`)

func clawhubPackageIncludes(rel string, ignorePatterns map[string]bool) bool {
	for pattern := range ignorePatterns {
		switch {
		case strings.HasSuffix(pattern, "/"):
			if strings.HasPrefix(rel, pattern) {
				return false
			}
		case strings.Contains(pattern, "*"):
			if matched, _ := path.Match(pattern, path.Base(rel)); matched {
				return false
			}
		case rel == pattern:
			return false
		}
	}
	return true
}

func TestClawhubPackageIncludesLogic(t *testing.T) {
	ignore := map[string]bool{"*_test.go": true, "testdata/": true, "AGENTS.md": true}
	for rel, want := range map[string]bool{
		"internal/auth/tokencache.go":      true,
		"internal/auth/tokencache_test.go": false,
		"testdata/x.json":                  false,
		"AGENTS.md":                        false,
		"SKILL.md":                         true,
	} {
		if got := clawhubPackageIncludes(rel, ignore); got != want {
			t.Errorf("clawhubPackageIncludes(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestClawhubPackageAvoidsCredentialAccessProse(t *testing.T) {
	if !skillSpectorCredentialAccessProse.MatchString("the on-disk access token") {
		t.Fatal("credential-access matcher no longer fires on the phrase SkillSpector flags")
	}
	root := repoRoot(t)
	ignore := clawhubIgnorePatterns(t)
	walkErr := filepath.WalkDir(root, func(full string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && full != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "bin", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(root, full)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if !clawhubPackageIncludes(rel, ignore) {
			return nil
		}
		raw, rerr := os.ReadFile(full)
		if rerr != nil {
			return rerr
		}
		for i, line := range strings.Split(string(raw), "\n") {
			if skillSpectorCredentialAccessProse.MatchString(line) {
				t.Errorf("%s:%d: %q matches SkillSpector's PE3 credential-access prose rule and is reported as a High Privilege Escalation finding; say \"OAuth token\" instead", rel, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk repo: %v", walkErr)
	}
}
