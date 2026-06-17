package config

import (
	"os"
	"path/filepath"
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

func TestDefaults(t *testing.T) {
	// An empty CWD with no config.json and no env: pure defaults.
	t.Chdir(t.TempDir())
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
	if cfg.Zone2Low != DefaultZone2Low || cfg.Zone2High != DefaultZone2High {
		t.Errorf("zone2 = %d-%d, want %d-%d", cfg.Zone2Low, cfg.Zone2High, DefaultZone2Low, DefaultZone2High)
	}
	if len(cfg.EllipticalTypes) != 1 || cfg.EllipticalTypes[0] != "ELLIPTICAL" {
		t.Errorf("EllipticalTypes = %v, want [ELLIPTICAL]", cfg.EllipticalTypes)
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != DefaultScope {
		t.Errorf("Scopes = %v, want [%s]", cfg.Scopes, DefaultScope)
	}
	if cfg.DailyLog != "" {
		t.Errorf("DailyLog = %q, want empty", cfg.DailyLog)
	}
	if err := cfg.RequireDailyLog(); err == nil {
		t.Error("RequireDailyLog() = nil, want error when daily_log empty")
	}
}

func TestFileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfig(t, dir, `{
	  "daily_log": "/some/DAILY_LOG.json",
	  "elliptical_types": ["ELLIPTICAL", "ROWING"],
	  "zone2_low": 100,
	  "zone2_high": 140,
	  "client_id": "cid-from-file",
	  "base_url": "https://example.test",
	  "user": "users/42",
	  "ghealth_bin": "should-be-ignored"
	}`)
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DailyLog != "/some/DAILY_LOG.json" {
		t.Errorf("DailyLog = %q", cfg.DailyLog)
	}
	if len(cfg.EllipticalTypes) != 2 || cfg.EllipticalTypes[1] != "ROWING" {
		t.Errorf("EllipticalTypes = %v", cfg.EllipticalTypes)
	}
	if cfg.Zone2Low != 100 || cfg.Zone2High != 140 {
		t.Errorf("zone2 = %d-%d", cfg.Zone2Low, cfg.Zone2High)
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
	if err := cfg.RequireDailyLog(); err != nil {
		t.Errorf("RequireDailyLog() = %v, want nil", err)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfig(t, dir, `{"daily_log": "/from/file.json", "client_id": "file-cid", "base_url": "https://file.test"}`)

	t.Setenv(EnvDailyLog, "/from/env.json")
	t.Setenv(EnvClientID, "env-cid")
	t.Setenv(EnvClientSecret, "env-secret")
	t.Setenv(EnvBaseURL, "https://env.test")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DailyLog != "/from/env.json" {
		t.Errorf("DailyLog = %q, want env value", cfg.DailyLog)
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
	writeConfig(t, cwd, `{"daily_log": "/cwd.json"}`)

	other := t.TempDir()
	otherPath := writeConfig(t, other, `{"daily_log": "/flag.json"}`)

	// Env config should lose to the explicit flag path.
	t.Setenv(EnvConfig, filepath.Join(cwd, "config.json"))

	cfg, err := Load(Options{ConfigPath: otherPath})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ConfigPath != otherPath {
		t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, otherPath)
	}
	if cfg.DailyLog != "/flag.json" {
		t.Errorf("DailyLog = %q, want /flag.json", cfg.DailyLog)
	}
}

func TestEnvConfigSelectsFile(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	// No config.json in CWD; GOOGLE_HEALTH_CONFIG points at another file.
	other := t.TempDir()
	otherPath := writeConfig(t, other, `{"daily_log": "/env-config.json"}`)
	t.Setenv(EnvConfig, otherPath)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DailyLog != "/env-config.json" {
		t.Errorf("DailyLog = %q, want /env-config.json", cfg.DailyLog)
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
