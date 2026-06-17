// Package config resolves runtime configuration and credential discovery.
//
// Precedence (GOAL.md §7), lowest to highest: built-in defaults < config.json <
// environment variables < command flags. The names of every env var and every
// JSON key are part of the external contract and must not be renamed — existing
// users and the Monday check-in automation depend on them.
//
// This is a port of the Python google_health/config.py, extended for the
// credentials the self-contained Go tool now owns (the Python tool held none —
// the `ghealth` binary did). The removed `ghealth_bin` / GOOGLE_HEALTH_BIN keys
// are ignored silently if a stale config still carries them (loose decode).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Environment variable names (frozen — GOAL.md §7). G101: these are env-var
// names, not embedded credentials.
const (
	EnvConfig       = "GOOGLE_HEALTH_CONFIG"
	EnvDailyLog     = "GOOGLE_HEALTH_DAILY_LOG"
	EnvClientID     = "GOOGLE_HEALTH_CLIENT_ID"
	EnvClientSecret = "GOOGLE_HEALTH_CLIENT_SECRET" //nolint:gosec // env var name, not a secret.
	EnvBaseURL      = "GOOGLE_HEALTH_BASE_URL"
	EnvTokenCache   = "GOOGLE_HEALTH_TOKEN_CACHE"
)

// Defaults (GOAL.md §7).
const (
	DefaultBaseURL   = "https://health.googleapis.com"
	DefaultUser      = "users/me"
	DefaultZone2Low  = 110
	DefaultZone2High = 130

	// DefaultScope is the single least-privilege read scope (GOAL.md §2.7, §22).
	DefaultScope = "https://www.googleapis.com/auth/googlehealth.activity_and_fitness.readonly"

	defaultConfigName = "config.json"
	defaultTokenName  = "token.json"
	appDirName        = "google-health-cli"
)

// ErrMissingDailyLog is returned by RequireDailyLog when no daily_log path could
// be resolved from any source.
var ErrMissingDailyLog = errors.New("missing daily_log path")

// Config is the fully resolved configuration handed to commands.
type Config struct {
	DailyLog        string
	EllipticalTypes []string
	Zone2Low        int
	Zone2High       int
	ClientID        string
	ClientSecret    string
	BaseURL         string
	User            string
	TokenCache      string
	Scopes          []string

	// ConfigPath is where config.json was loaded from, or where it would be
	// written if it does not yet exist.
	ConfigPath string
	// ConfigExists reports whether ConfigPath was present on disk at load time.
	ConfigExists bool
}

// fileConfig mirrors config.json. Pointer fields distinguish "key present" from
// "key absent" so an absent key falls through to the default rather than
// overwriting it with a zero value. Unknown keys (e.g. the removed ghealth_bin)
// are ignored by encoding/json — exactly the "loose decode" GOAL.md §7 wants.
type fileConfig struct {
	DailyLog        *string  `json:"daily_log"`
	EllipticalTypes []string `json:"elliptical_types"`
	Zone2Low        *int     `json:"zone2_low"`
	Zone2High       *int     `json:"zone2_high"`
	ClientID        *string  `json:"client_id"`
	ClientSecret    *string  `json:"client_secret"`
	BaseURL         *string  `json:"base_url"`
	User            *string  `json:"user"`
	TokenCache      *string  `json:"token_cache"`
	Scopes          []string `json:"scopes"`
}

// Options carries inputs the caller already knows from flags, so config
// resolution can honor flag precedence without importing cobra.
type Options struct {
	// ConfigPath is the value of the --config flag (empty if not set). It wins
	// over GOOGLE_HEALTH_CONFIG when choosing which file to read.
	ConfigPath string
}

// Load resolves configuration from defaults, config.json, and environment
// variables (GOAL.md §7).
func Load(opts Options) (*Config, error) {
	cfg := &Config{
		EllipticalTypes: []string{"ELLIPTICAL"},
		Zone2Low:        DefaultZone2Low,
		Zone2High:       DefaultZone2High,
		BaseURL:         DefaultBaseURL,
		User:            DefaultUser,
		Scopes:          []string{DefaultScope},
	}

	// 1. Locate and read config.json (if any).
	path := discoverConfigPath(opts.ConfigPath)
	cfg.ConfigPath = path

	fc, exists, err := readFileConfig(path)
	if err != nil {
		return nil, err
	}
	cfg.ConfigExists = exists
	applyFile(cfg, fc)

	// 2. Environment overrides (LookupEnv distinguishes set-empty from unset).
	applyEnv(cfg)

	// 3. Token cache location: env override, else <UserConfigDir>/app/token.json.
	if cfg.TokenCache == "" {
		cfg.TokenCache = defaultTokenCachePath()
	}

	return cfg, nil
}

// discoverConfigPath implements GOAL.md §7 discovery:
//  1. --config flag or GOOGLE_HEALTH_CONFIG env (explicit path; flag wins).
//  2. config.json in the current working directory.
//  3. config.json next to the executable — the analog of Python's "next to the
//     package" fallback, so the Monday check-in can run from the Workout folder.
//  4. otherwise keep the CWD path (so `config show`/errors have something sane).
func discoverConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if v, ok := os.LookupEnv(EnvConfig); ok && v != "" {
		return v
	}
	if _, err := os.Stat(defaultConfigName); err == nil {
		return defaultConfigName
	}
	if exe, err := os.Executable(); err == nil {
		alt := filepath.Join(filepath.Dir(exe), defaultConfigName)
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return defaultConfigName
}

func readFileConfig(path string) (fileConfig, bool, error) {
	var fc fileConfig
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fc, false, nil
		}
		return fc, false, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, true, fmt.Errorf("parse config %s: %w", path, err)
	}
	return fc, true, nil
}

func applyFile(cfg *Config, fc fileConfig) {
	if fc.DailyLog != nil {
		cfg.DailyLog = *fc.DailyLog
	}
	if fc.EllipticalTypes != nil {
		cfg.EllipticalTypes = fc.EllipticalTypes
	}
	if fc.Zone2Low != nil {
		cfg.Zone2Low = *fc.Zone2Low
	}
	if fc.Zone2High != nil {
		cfg.Zone2High = *fc.Zone2High
	}
	if fc.ClientID != nil {
		cfg.ClientID = *fc.ClientID
	}
	if fc.ClientSecret != nil {
		cfg.ClientSecret = *fc.ClientSecret
	}
	if fc.BaseURL != nil {
		cfg.BaseURL = *fc.BaseURL
	}
	if fc.User != nil {
		cfg.User = *fc.User
	}
	if fc.TokenCache != nil {
		cfg.TokenCache = *fc.TokenCache
	}
	if fc.Scopes != nil {
		cfg.Scopes = fc.Scopes
	}
}

func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv(EnvDailyLog); ok {
		cfg.DailyLog = v
	}
	if v, ok := os.LookupEnv(EnvClientID); ok {
		cfg.ClientID = v
	}
	if v, ok := os.LookupEnv(EnvClientSecret); ok {
		cfg.ClientSecret = v
	}
	if v, ok := os.LookupEnv(EnvBaseURL); ok {
		cfg.BaseURL = v
	}
	if v, ok := os.LookupEnv(EnvTokenCache); ok {
		cfg.TokenCache = v
	}
}

// defaultTokenCachePath is <UserConfigDir>/google-health-cli/token.json, falling
// back to ./token.json if the user config dir can't be determined.
func defaultTokenCachePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, appDirName, defaultTokenName)
	}
	return defaultTokenName
}

// RequireDailyLog returns a friendly error (shown on stderr) when no daily_log
// path was resolved. Mirrors the Python config.py SystemExit message (GOAL.md
// §7, §12).
func (c *Config) RequireDailyLog() error {
	if c.DailyLog == "" {
		return fmt.Errorf("%w: set it in config.json or via %s. "+
			"It should point at the Workout project's DAILY_LOG.json", ErrMissingDailyLog, EnvDailyLog)
	}
	return nil
}
