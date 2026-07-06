package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stozo04/google-health-cli/internal/config"
)

// guardedImports lists import paths that grant process execution or dynamic
// code loading, each mapped to the (slash-separated, repo-relative) source files
// permitted to use it. A read-only data collector must spawn no processes and
// load no code at runtime; the one sanctioned exception is opening the browser
// for the interactive OAuth login. This is the dedicated guard for the
// Behavioral AST / Excessive Agency categories in .claude/CLAWHUB_STANDARDS.md.
var guardedImports = map[string][]string{
	"os/exec": {"internal/auth/oauth.go"}, // OpenBrowser launches the OAuth consent page
	"plugin":  nil,                        // dynamic code loading — never allowed
	"unsafe":  nil,                        // memory reinterpretation — never allowed
}

// guardedImportViolation reports whether importing path from the file at rel
// (repo-relative, forward slashes) is disallowed.
func guardedImportViolation(rel, path string) bool {
	allowed, watched := guardedImports[path]
	if !watched {
		return false
	}
	for _, f := range allowed {
		if f == rel {
			return false
		}
	}
	return true
}

// TestNoUndeclaredProcessExecutionOrDynamicCode walks the shipped (non-test) Go
// source and fails if any file outside the allowlist imports a process-spawning
// or dynamic-code-loading package. Fix a failure by removing the import — never
// by widening the allowlist to dodge it. If a new exec site is genuinely
// required, justify it, add it to guardedImports, and record it in the standards.
func TestNoUndeclaredProcessExecutionOrDynamicCode(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", "dist", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if guardedImportViolation(rel, p) {
				t.Errorf("%s imports %q, which is not sanctioned in shipped code. A read-only "+
					"collector must not spawn processes or load code at runtime; the only allowed "+
					"exec is the OAuth browser-open in internal/auth/oauth.go. If this is intentional, "+
					"justify it, extend guardedImports here, and update .claude/CLAWHUB_STANDARDS.md.",
					rel, p)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk source tree: %v", walkErr)
	}
}

// TestGuardedImportViolationLogic proves the matcher can actually fail — an
// immutable guard that cannot fire is worthless. A non-allowlisted file using a
// guarded import is a violation; the allowlisted file is not; an unwatched import
// never is.
func TestGuardedImportViolationLogic(t *testing.T) {
	cases := []struct {
		rel, path string
		want      bool
	}{
		{"internal/cli/data.go", "os/exec", true},
		{"internal/auth/oauth.go", "os/exec", false},
		{"internal/cli/data.go", "plugin", true},
		{"internal/cli/data.go", "unsafe", true},
		{"internal/auth/oauth.go", "plugin", true},
		{"internal/cli/data.go", "fmt", false},
	}
	for _, c := range cases {
		if got := guardedImportViolation(c.rel, c.path); got != c.want {
			t.Errorf("guardedImportViolation(%q, %q) = %v, want %v", c.rel, c.path, got, c.want)
		}
	}
}

// declaredEnvVars is the closed set of environment variables the shipped
// binary may read — exactly the GOOGLE_HEALTH_* keys advertised in SKILL.md's
// envVars block. CLAWHUB_STANDARDS (Data Exfiltration) promises "env reads are
// the specific GOOGLE_HEALTH_* keys via os.LookupEnv"; this guard makes that
// promise fail the build instead of drifting (advertised == actual, both ways).
var declaredEnvVars = map[string]bool{
	config.EnvConfig:       true,
	config.EnvClientID:     true,
	config.EnvClientSecret: true,
	config.EnvBaseURL:      true,
	config.EnvTokenCache:   true,
}

// envConstIdents are the exported config constants naming those variables —
// the only non-literal argument forms an env-read call may use, so every read
// site remains statically verifiable against declaredEnvVars.
var envConstIdents = map[string]bool{
	"EnvConfig":       true,
	"EnvClientID":     true,
	"EnvClientSecret": true,
	"EnvBaseURL":      true,
	"EnvTokenCache":   true,
}

// allowedEnvRead reports whether shipped code may read the named variable.
func allowedEnvRead(name string) bool { return declaredEnvVars[name] }

// TestShippedSourceReadsOnlyDeclaredEnvVars walks the shipped (non-test) Go
// source and fails on any environment read outside the declared set: an
// os.Environ sweep, an os.Getenv/LookupEnv of an unadvertised name (this caught
// a stray LOG_LEVEL read), or an argument too dynamic to verify. It then checks
// the advertisement side: every declared variable must appear in SKILL.md. Fix
// a failure by removing the read or declaring the variable in SKILL.md AND
// here — never by deleting the test.
func TestShippedSourceReadsOnlyDeclaredEnvVars(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", "dist", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "os" {
				return true
			}
			switch sel.Sel.Name {
			case "Environ":
				t.Errorf("%s calls os.Environ(): a full environment sweep is never allowed (Data Exfiltration)", rel)
			case "Getenv", "LookupEnv":
				if len(call.Args) != 1 {
					return true
				}
				switch arg := call.Args[0].(type) {
				case *ast.BasicLit:
					if name, uerr := strconv.Unquote(arg.Value); uerr == nil && !allowedEnvRead(name) {
						t.Errorf("%s reads undeclared env var %q; declare it in SKILL.md envVars and declaredEnvVars, or remove the read", rel, name)
					}
				case *ast.Ident:
					if !envConstIdents[arg.Name] {
						t.Errorf("%s reads env via unrecognized identifier %s; use a declared Env* constant", rel, arg.Name)
					}
				case *ast.SelectorExpr:
					if !envConstIdents[arg.Sel.Name] {
						t.Errorf("%s reads env via unrecognized selector %s; use a declared config.Env* constant", rel, arg.Sel.Name)
					}
				default:
					t.Errorf("%s reads env with a dynamic argument; env reads must be statically verifiable", rel)
				}
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk source tree: %v", walkErr)
	}

	// Advertisement side: every variable the binary may read is documented.
	skill, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	for name := range declaredEnvVars {
		if !strings.Contains(string(skill), name) {
			t.Errorf("SKILL.md does not advertise env var %s, but the binary may read it", name)
		}
	}
}

// TestAllowedEnvReadLogic proves the env-read matcher can actually fire: the
// declared keys pass, anything else (e.g. the LOG_LEVEL read this guard was
// added to catch) is a violation.
func TestAllowedEnvReadLogic(t *testing.T) {
	for name, want := range map[string]bool{
		config.EnvConfig:      true,
		config.EnvTokenCache:  true,
		"LOG_LEVEL":           false,
		"PATH":                false,
		"GOOGLE_HEALTH_OTHER": false,
	} {
		if got := allowedEnvRead(name); got != want {
			t.Errorf("allowedEnvRead(%q) = %v, want %v", name, got, want)
		}
	}
}

// hasReplaceDirective reports whether go.mod content carries any `replace`
// directive (single-line or block form). A replace can redirect a pinned
// dependency to a local path or fork that go.sum checksums don't vouch for — a
// supply-chain redirection. This self-contained CLI uses none.
func hasReplaceDirective(goMod string) bool {
	for _, line := range strings.Split(goMod, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "replace" {
			return true
		}
	}
	return false
}

// TestGoModHasNoReplaceDirectives is the dedicated Supply Chain guard: it fails
// if go.mod redirects any dependency. (go.sum checksums are separately enforced
// by the toolchain on every CI build.)
func TestGoModHasNoReplaceDirectives(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if hasReplaceDirective(string(raw)) {
		t.Error("go.mod contains a replace directive: a replace can redirect a pinned dependency " +
			"to an unvetted local path or fork. Remove it, or if it is genuinely required, document " +
			"the supply-chain justification and update .claude/CLAWHUB_STANDARDS.md.")
	}
}

// TestReplaceDirectiveDetection proves hasReplaceDirective fires on the directive
// forms and ignores look-alikes (a bare mention in a comment).
func TestReplaceDirectiveDetection(t *testing.T) {
	withReplace := []string{
		"replace example.com/x => ./local",
		"  replace example.com/x => example.com/y v1.2.3",
		"require a v1.0.0\nreplace (\n\texample.com/x => ./local\n)\n",
	}
	clean := []string{
		"module m\n\ngo 1.25\n\nrequire (\n\tx v1.0.0\n)\n",
		"// replace is only mentioned in a comment here\n",
	}
	for _, s := range withReplace {
		if !hasReplaceDirective(s) {
			t.Errorf("hasReplaceDirective(%q) = false, want true", s)
		}
	}
	for _, s := range clean {
		if hasReplaceDirective(s) {
			t.Errorf("hasReplaceDirective(%q) = true, want false", s)
		}
	}
}
