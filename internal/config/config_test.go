package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a config.json into dir and returns its path.
func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// isolateAppDir points discovery's user-config-dir at an empty temp dir so the
// appdir fallback never picks up the developer's real ~/.config or %AppData%
// config.json during tests. It returns the temp dir so callers can plant a
// config under <dir>/google-health-cli/ to exercise appdir discovery.
func isolateAppDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = prev })
	return dir
}

func TestDefaults(t *testing.T) {
	// An empty CWD with no config.json and no env: pure defaults. Isolate the
	// appdir fallback so a real user-config-dir config can't leak in.
	t.Chdir(t.TempDir())
	isolateAppDir(t)
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.BaseURL; got != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", got, DefaultBaseURL)
	}
	if got := cfg.User; got != DefaultUser {
		t.Errorf("User = %q, want %q", got, DefaultUser)
	}
	if len(cfg.Scopes) != len(DefaultScopes) {
		t.Errorf("Scopes = %v, want the %d default read scopes", cfg.Scopes, len(DefaultScopes))
	}
	for i, s := range DefaultScopes {
		if cfg.Scopes[i] != s {
			t.Errorf("Scopes[%d] = %q, want %q", i, cfg.Scopes[i], s)
		}
	}
}

func TestFileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// A stale daily_log / elliptical_types is silently ignored (loose decode).
	writeConfig(t, dir, `{
	  "client_id": "cid-from-file",
	  "base_url": "https://example.test",
	  "user": "users/42",
	  "scopes": ["https://www.googleapis.com/auth/googlehealth.sleep.readonly"],
	  "daily_log": "should-be-ignored",
	  "elliptical_types": ["ELLIPTICAL"]
	}`)
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientID != "cid-from-file" {
		t.Errorf("ClientID = %q", cfg.ClientID)
	}
	if cfg.BaseURL != "https://example.test" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.User != "users/42" {
		t.Errorf("User = %q", cfg.User)
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != "https://www.googleapis.com/auth/googlehealth.sleep.readonly" {
		t.Errorf("Scopes = %v, want the single file-provided scope", cfg.Scopes)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfig(t, dir, `{"client_id": "file-cid", "base_url": "https://file.test"}`)

	t.Setenv(EnvClientID, "env-cid")
	t.Setenv(EnvClientSecret, "env-secret")
	t.Setenv(EnvBaseURL, "https://env.test")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientID != "env-cid" {
		t.Errorf("ClientID = %q, want env value", cfg.ClientID)
	}
	if cfg.ClientSecret != "env-secret" {
		t.Errorf("ClientSecret = %q, want env value", cfg.ClientSecret)
	}
	if cfg.BaseURL != "https://env.test" {
		t.Errorf("BaseURL = %q, want env value", cfg.BaseURL)
	}
}

func TestFlagConfigPathWins(t *testing.T) {
	// A config.json in CWD that should be ignored when --config points elsewhere.
	cwd := t.TempDir()
	t.Chdir(cwd)
	writeConfig(t, cwd, `{"client_id": "cwd-cid"}`)

	other := t.TempDir()
	otherPath := writeConfig(t, other, `{"client_id": "flag-cid"}`)

	// Env config should lose to the explicit flag path.
	t.Setenv(EnvConfig, filepath.Join(cwd, "config.json"))

	cfg, err := Load(Options{ConfigPath: otherPath})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ConfigPath != otherPath {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, otherPath)
	}
	if cfg.ClientID != "flag-cid" {
		t.Errorf("ClientID = %q, want flag-cid", cfg.ClientID)
	}
}

func TestEnvConfigSelectsFile(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	// No config.json in CWD; GOOGLE_HEALTH_CONFIG points at another file.
	other := t.TempDir()
	otherPath := writeConfig(t, other, `{"client_id": "env-config-cid"}`)
	t.Setenv(EnvConfig, otherPath)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ClientID != "env-config-cid" {
		t.Errorf("ClientID = %q, want env-config-cid", cfg.ClientID)
	}
}

func TestTokenCacheEnvOverride(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EnvTokenCache, "/tmp/throwaway-token.json")
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TokenCache != "/tmp/throwaway-token.json" {
		t.Errorf("TokenCache = %q, want env value", cfg.TokenCache)
	}
}

func TestMalformedConfigIsError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfig(t, dir, `{not valid json`)
	if _, err := Load(Options{}); err == nil {
		t.Error("Load() = nil error, want parse error for malformed config")
	}
}

func TestAppDirConfigDiscovered(t *testing.T) {
	// No --config, no env, no CWD config: the config.json next to the token
	// cache (user config dir) must be auto-discovered (issue #6, AC #4).
	t.Chdir(t.TempDir())
	appBase := isolateAppDir(t)
	appDir := filepath.Join(appBase, appDirName)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatalf("mkdir appdir: %v", err)
	}
	writeConfig(t, appDir, `{"client_id": "appdir-cid", "client_secret": "appdir-secret"}`)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ConfigExists {
		t.Error("ConfigExists = false, want true for the appdir config")
	}
	if cfg.ClientID != "appdir-cid" {
		t.Errorf("ClientID = %q, want appdir-cid", cfg.ClientID)
	}
	if !cfg.HasOAuthClient() {
		t.Error("HasOAuthClient() = false, want true")
	}
}

func TestNoConfigRecordsSearchedPaths(t *testing.T) {
	// With nothing discoverable, ConfigExists is false and SearchedPaths names
	// every location consulted so callers can build a "looked at" hint.
	t.Chdir(t.TempDir())
	isolateAppDir(t)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ConfigExists {
		t.Error("ConfigExists = true, want false when no config is discoverable")
	}
	if cfg.HasOAuthClient() {
		t.Error("HasOAuthClient() = true, want false with no config")
	}
	joined := strings.Join(cfg.SearchedPaths, " | ")
	for _, want := range []string{EnvConfig, defaultConfigName, appDirName} {
		if !strings.Contains(joined, want) {
			t.Errorf("SearchedPaths %v missing %q", cfg.SearchedPaths, want)
		}
	}
}

func TestFlagPathRecordsSingleSearchedPath(t *testing.T) {
	// An explicit --config short-circuits discovery; SearchedPaths names just it.
	t.Chdir(t.TempDir())
	isolateAppDir(t)
	missing := filepath.Join(t.TempDir(), "nope.json")

	cfg, err := Load(Options{ConfigPath: missing})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ConfigExists {
		t.Error("ConfigExists = true, want false for a nonexistent --config path")
	}
	if len(cfg.SearchedPaths) != 1 || !strings.Contains(cfg.SearchedPaths[0], missing) {
		t.Errorf("SearchedPaths = %v, want a single entry naming %q", cfg.SearchedPaths, missing)
	}
}
