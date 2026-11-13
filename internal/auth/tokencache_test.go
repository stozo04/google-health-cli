package auth

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "token.json")
	want := &oauth2.Token{
		AccessToken:  "access-123",
		TokenType:    "Bearer",
		RefreshToken: "refresh-456",
		Expiry:       time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	}
	if err := SaveToken(path, want); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	got, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if got == nil {
		t.Fatal("LoadToken returned nil for a saved token")
	}
	if got.AccessToken != want.AccessToken || got.RefreshToken != want.RefreshToken || got.TokenType != want.TokenType {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if !got.Expiry.Equal(want.Expiry) {
		t.Errorf("Expiry = %v, want %v", got.Expiry, want.Expiry)
	}
}

func TestSaveTokenPerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are advisory on Windows")
	}
	path := filepath.Join(t.TempDir(), "token.json")
	if err := SaveToken(path, &oauth2.Token{AccessToken: "x"}); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}

func TestLoadMissingIsNil(t *testing.T) {
	got, err := LoadToken(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("LoadToken(missing) error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("LoadToken(missing) = %+v, want nil", got)
	}
}

func TestLoadCorruptIsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken(corrupt) error = %v, want nil (treat as absent)", err)
	}
	if got != nil {
		t.Errorf("LoadToken(corrupt) = %+v, want nil", got)
	}
}

func TestDeleteToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	if err := SaveToken(path, &oauth2.Token{AccessToken: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteToken(path); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("token still present after delete: %v", err)
	}
	// Deleting an already-absent file is not an error.
	if err := DeleteToken(path); err != nil {
		t.Errorf("DeleteToken(missing) = %v, want nil", err)
	}
}

func TestMigrateLegacyToken(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "roaming", "token.json")
	newPath := filepath.Join(dir, "local", "token.json")
	if err := SaveToken(legacy, &oauth2.Token{AccessToken: "a", RefreshToken: "r", TokenType: "Bearer"}); err != nil {
		t.Fatal(err)
	}

	moved, err := MigrateLegacyToken(newPath, legacy)
	if err != nil || !moved {
		t.Fatalf("MigrateLegacyToken: moved=%v err=%v, want true/nil", moved, err)
	}
	if _, serr := os.Stat(legacy); !errors.Is(serr, os.ErrNotExist) {
		t.Error("legacy token still present after migration")
	}
	got, err := LoadToken(newPath)
	if err != nil || got == nil || got.AccessToken != "a" {
		t.Fatalf("relocated token missing/wrong: %+v err=%v", got, err)
	}

	// Idempotent: a second run is a no-op (legacy is gone).
	if moved2, _ := MigrateLegacyToken(newPath, legacy); moved2 {
		t.Error("second migration reported a move, want no-op")
	}
}

func TestMigrateLegacyTokenNeverClobbers(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "roaming", "token.json")
	newPath := filepath.Join(dir, "local", "token.json")
	if err := SaveToken(legacy, &oauth2.Token{AccessToken: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveToken(newPath, &oauth2.Token{AccessToken: "current"}); err != nil {
		t.Fatal(err)
	}

	moved, err := MigrateLegacyToken(newPath, legacy)
	if err != nil || moved {
		t.Fatalf("MigrateLegacyToken: moved=%v err=%v, want false/nil (destination occupied)", moved, err)
	}
	got, _ := LoadToken(newPath)
	if got == nil || got.AccessToken != "current" {
		t.Errorf("destination token was clobbered: %+v", got)
	}
	if _, serr := os.Stat(legacy); errors.Is(serr, os.ErrNotExist) {
		t.Error("legacy token was removed despite no migration")
	}
}

func TestMigrateLegacyTokenNoLegacy(t *testing.T) {
	dir := t.TempDir()
	moved, err := MigrateLegacyToken(filepath.Join(dir, "new.json"), filepath.Join(dir, "absent.json"))
	if err != nil || moved {
		t.Fatalf("MigrateLegacyToken(no legacy): moved=%v err=%v, want false/nil", moved, err)
	}
}

// staticSource returns a fixed token, simulating a refresh that produced it.
type staticSource struct{ tok *oauth2.Token }

func (s staticSource) Token() (*oauth2.Token, error) { return s.tok, nil }

func TestPersistingSourceWritesOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	old := &oauth2.Token{AccessToken: "old", Expiry: time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)}
	refreshed := &oauth2.Token{AccessToken: "new", Expiry: time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC)}

	ps := &persistingSource{
		base:   staticSource{refreshed},
		path:   path,
		last:   old,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	tok, err := ps.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "new" {
		t.Errorf("Token().AccessToken = %q, want new", tok.AccessToken)
	}
	saved, err := LoadToken(path)
	if err != nil || saved == nil {
		t.Fatalf("expected persisted token, got %+v err=%v", saved, err)
	}
	if saved.AccessToken != "new" {
		t.Errorf("persisted AccessToken = %q, want new", saved.AccessToken)
	}
}

func TestPersistingSourceSkipsWhenUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	same := &oauth2.Token{AccessToken: "same", Expiry: time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)}

	ps := &persistingSource{
		base:   staticSource{same},
		path:   path,
		last:   same,
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	if _, err := ps.Token(); err != nil {
		t.Fatalf("Token: %v", err)
	}
	// Unchanged token must not write the cache.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cache written despite no change: %v", err)
	}
}

const decoyContent = "-----BEGIN OPENSSH PRIVATE KEY-----\nnot-a-real-key\n"

func writeDecoy(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, []byte(decoyContent), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertDecoyIntact(t *testing.T, path string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("decoy file gone: %v", err)
	}
	if string(got) != decoyContent {
		t.Errorf("decoy file was rewritten: %q", got)
	}
}

func TestSaveTokenRefusesToOverwriteNonTokenFile(t *testing.T) {
	path := writeDecoy(t)
	err := SaveToken(path, &oauth2.Token{AccessToken: "x"})
	if !errors.Is(err, ErrNotTokenCache) {
		t.Errorf("SaveToken over a non-token file: err = %v, want ErrNotTokenCache", err)
	}
	assertDecoyIntact(t, path)
}

func TestDeleteTokenRefusesToDeleteNonTokenFile(t *testing.T) {
	path := writeDecoy(t)
	if err := DeleteToken(path); !errors.Is(err, ErrNotTokenCache) {
		t.Errorf("DeleteToken on a non-token file: err = %v, want ErrNotTokenCache", err)
	}
	assertDecoyIntact(t, path)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSaveTokenReplacesOnlyCachesThisToolOwns(t *testing.T) {
	const foreignToken = `{"access_token":"other-app","refresh_token":"other-refresh","token_type":"Bearer"}`
	cases := []struct {
		name    string
		path    func(dir string) string
		setup   func(t *testing.T, path string)
		allowed bool
	}{
		{
			"our marked cache at a custom path", func(d string) string { return filepath.Join(d, "custom.json") },
			func(t *testing.T, p string) {
				if err := SaveToken(p, &oauth2.Token{AccessToken: "old"}); err != nil {
					t.Fatal(err)
				}
			}, true,
		},
		{
			"empty file", func(d string) string { return filepath.Join(d, "custom.json") },
			func(t *testing.T, p string) { writeFile(t, p, "") }, false,
		},
		{"unmarked token at the conventional path", func(d string) string {
			return filepath.Join(d, "google-health-cli", "token.json")
		}, func(t *testing.T, p string) { writeFile(t, p, foreignToken) }, true},
		{
			"another app's token.json", func(d string) string { return filepath.Join(d, "other-app", "token.json") },
			func(t *testing.T, p string) { writeFile(t, p, foreignToken) }, false,
		},
		{
			"oversized file", func(d string) string { return filepath.Join(d, "custom.json") },
			func(t *testing.T, p string) { writeFile(t, p, strings.Repeat(" ", maxTokenCacheBytes+1)) }, false,
		},
		{
			"directory", func(d string) string { return filepath.Join(d, "token.json") },
			func(t *testing.T, p string) {
				if err := os.MkdirAll(p, 0o700); err != nil {
					t.Fatal(err)
				}
			}, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := c.path(t.TempDir())
			c.setup(t, path)
			err := SaveToken(path, &oauth2.Token{AccessToken: "new"})
			if c.allowed {
				if err != nil {
					t.Fatalf("SaveToken: %v", err)
				}
				got, lerr := LoadToken(path)
				if lerr != nil || got == nil || got.AccessToken != "new" {
					t.Errorf("LoadToken after save = %+v, %v; want access token %q", got, lerr, "new")
				}
				return
			}
			if !errors.Is(err, ErrNotTokenCache) {
				t.Errorf("SaveToken: err = %v, want ErrNotTokenCache", err)
			}
			if derr := DeleteToken(path); !errors.Is(derr, ErrNotTokenCache) {
				t.Errorf("DeleteToken: err = %v, want ErrNotTokenCache", derr)
			}
			if _, serr := os.Stat(path); serr != nil {
				t.Errorf("refused target was removed: %v", serr)
			}
		})
	}
}

func TestSaveTokenMarksCacheAsOwned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.json")
	if err := SaveToken(path, &oauth2.Token{AccessToken: "x"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"owner": "google-health-cli"`) {
		t.Errorf("token cache lacks the ownership marker: %s", raw)
	}
}

func TestTokenCacheRefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs extra privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "other-app", "token.json")
	writeFile(t, target, `{"access_token":"other-app","refresh_token":"other-refresh"}`)
	link := filepath.Join(dir, "google-health-cli", "token.json")
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveToken(link, &oauth2.Token{AccessToken: "new"}); !errors.Is(err, ErrNotTokenCache) {
		t.Errorf("SaveToken through a symlink: err = %v, want ErrNotTokenCache", err)
	}
	if err := DeleteToken(link); !errors.Is(err, ErrNotTokenCache) {
		t.Errorf("DeleteToken on a symlink: err = %v, want ErrNotTokenCache", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(got), "other-app") {
		t.Errorf("symlink target was changed: %q, %v", got, err)
	}
}

func TestRefreshRewritesUnmarkedCacheOnlyWithItsOwnRefreshToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.json")
	writeFile(t, path, `{"access_token":"old","refresh_token":"ours","token_type":"Bearer"}`)

	if err := saveToken(path, &oauth2.Token{AccessToken: "x"}, "someone-elses"); !errors.Is(err, ErrNotTokenCache) {
		t.Errorf("save with a different proven refresh token: err = %v, want ErrNotTokenCache", err)
	}
	if err := saveToken(path, &oauth2.Token{AccessToken: "fresh", RefreshToken: "ours"}, "ours"); err != nil {
		t.Fatalf("save after refreshing with the cache's own refresh token: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"owner": "google-health-cli"`) || !strings.Contains(string(raw), "fresh") {
		t.Errorf("refreshed cache should be rewritten with the owner marker: %s", raw)
	}
}

func TestLoadTokenRefusesUntrustedCacheTargets(t *testing.T) {
	dir := t.TempDir()
	token := filepath.Join(dir, "real", "token.json")
	writeFile(t, token, `{"access_token":"a","refresh_token":"r"}`)
	oversized := filepath.Join(dir, "big.json")
	writeFile(t, oversized, `{"access_token":"a","pad":"`+strings.Repeat("x", maxTokenCacheBytes)+`"}`)

	targets := map[string]string{
		"directory": dir,
		"oversized": oversized,
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.json")
		if err := os.Symlink(token, link); err != nil {
			t.Fatal(err)
		}
		targets["symlink"] = link
		targets["endless device"] = "/dev/zero"
	}
	for name, path := range targets {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			var tok *oauth2.Token
			var err error
			go func() {
				tok, err = LoadToken(path)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatalf("LoadToken(%s) did not return; an untrusted token_cache must never block", path)
			}
			if tok != nil || !errors.Is(err, ErrNotTokenCache) {
				t.Errorf("LoadToken(%s) = %v, %v; want nil, ErrNotTokenCache", path, tok, err)
			}
		})
	}
}
