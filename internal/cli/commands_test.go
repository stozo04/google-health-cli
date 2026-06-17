package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/stozo04/google-health-cli/internal/auth"
)

// fixtureServer serves the committed exercise fixture for any request.
func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "exercise_all.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// withBaseURL points the API client at url for the duration of the test.
func withBaseURL(t *testing.T, url string) {
	t.Helper()
	prev := baseURLOverride
	baseURLOverride = url
	t.Cleanup(func() { baseURLOverride = prev })
}

// testConfig writes a config.json and (optionally) a valid token into a temp
// dir, returning the config path and daily-log path.
func testConfig(t *testing.T, withToken bool) (cfgPath, dailyLog string) {
	t.Helper()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	dailyLog = filepath.Join(dir, "DAILY_LOG.json")
	cfgPath = filepath.Join(dir, "config.json")

	cfg := map[string]any{
		"daily_log":     dailyLog,
		"client_id":     "test-client",
		"client_secret": "test-secret",
		"token_cache":   tokenPath,
	}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if withToken {
		tok := &oauth2.Token{AccessToken: "fake", TokenType: "Bearer", Expiry: time.Now().Add(24 * time.Hour)}
		if err := auth.SaveToken(tokenPath, tok); err != nil {
			t.Fatal(err)
		}
	}
	return cfgPath, dailyLog
}

// run executes the root command with args, capturing stdout and stderr.
func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errBuf.String(), err
}

func TestSessionsJSONCommand(t *testing.T) {
	srv := fixtureServer(t)
	withBaseURL(t, srv.URL)
	cfg, _ := testConfig(t, true)

	stdout, _, err := run(t, "--config", cfg, "sessions", "--json", "--date", "2026-06-16", "--days", "60")
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	assertGolden(t, "sessions_json.golden", []byte(stdout))
}

func TestSessionsRawCommand(t *testing.T) {
	srv := fixtureServer(t)
	withBaseURL(t, srv.URL)
	cfg, _ := testConfig(t, true)

	stdout, _, err := run(t, "--config", cfg, "sessions", "--raw", "--date", "2026-06-16", "--days", "60")
	if err != nil {
		t.Fatalf("sessions --raw: %v", err)
	}
	assertGolden(t, "sessions_raw.golden", []byte(stdout))
}

func TestSessionsNotAuthenticated(t *testing.T) {
	cfg, _ := testConfig(t, false) // no token
	_, _, err := run(t, "--config", cfg, "sessions")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitAuth {
		t.Fatalf("err = %v, want ExitError code 2", err)
	}
}

func TestSyncCommandWritesGolden(t *testing.T) {
	srv := fixtureServer(t)
	withBaseURL(t, srv.URL)
	cfg, dailyLog := testConfig(t, true)

	// Seed the daily log with the committed input fixture.
	in, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "daily_log_input.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dailyLog, in, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := run(t, "--config", cfg, "sync", "--date", "2026-06-16", "--days", "30")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	assertGolden(t, "sync_human.golden", []byte(stdout))

	written, err := os.ReadFile(dailyLog)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "daily_log_output.golden", written)
}

func TestSyncJSONCommand(t *testing.T) {
	srv := fixtureServer(t)
	withBaseURL(t, srv.URL)
	cfg, dailyLog := testConfig(t, true)
	in, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "daily_log_input.json"))
	if err := os.WriteFile(dailyLog, in, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := run(t, "--config", cfg, "sync", "--json", "--date", "2026-06-16", "--days", "30")
	if err != nil {
		t.Fatalf("sync --json: %v", err)
	}
	var summary syncSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("sync --json not valid JSON: %v\n%s", err, stdout)
	}
	if summary.Target != "2026-06-16" || summary.Days != 30 || summary.DryRun {
		t.Errorf("summary header wrong: %+v", summary)
	}
	if summary.DroppedNonCardio != 5 {
		t.Errorf("dropped_non_cardio = %d, want 5", summary.DroppedNonCardio)
	}
	if len(summary.Results) != 4 {
		t.Fatalf("results = %d, want 4", len(summary.Results))
	}
	statuses := map[string]string{}
	for _, r := range summary.Results {
		statuses[r.Date] = r.Status
	}
	if statuses["2026-06-16"] != "conflict" || statuses["2026-06-02"] != "updated" {
		t.Errorf("statuses = %v", statuses)
	}
}

func TestSyncDryRunDoesNotWrite(t *testing.T) {
	srv := fixtureServer(t)
	withBaseURL(t, srv.URL)
	cfg, dailyLog := testConfig(t, true)
	in, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "daily_log_input.json"))
	if err := os.WriteFile(dailyLog, in, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := run(t, "--config", cfg, "sync", "--dry-run", "--date", "2026-06-16", "--days", "30")
	if err != nil {
		t.Fatalf("sync --dry-run: %v", err)
	}
	if !strings.HasPrefix(stdout, "[dry-run] would write") {
		t.Errorf("dry-run output missing tag: %q", stdout)
	}
	after, _ := os.ReadFile(dailyLog)
	if !bytes.Equal(after, in) {
		t.Error("dry-run modified the daily log")
	}
}

func TestDoctorValidToken(t *testing.T) {
	cfg, _ := testConfig(t, true)
	stdout, _, err := run(t, "--config", cfg, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	var report doctorReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("doctor JSON: %v\n%s", err, stdout)
	}
	if !report.TokenValid {
		t.Error("tokenValid = false, want true for a fresh token")
	}
}

func TestDoctorNotAuthenticatedExits2(t *testing.T) {
	cfg, _ := testConfig(t, false)
	stdout, stderr, err := run(t, "--config", cfg, "doctor")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitAuth {
		t.Fatalf("err = %v, want exit 2", err)
	}
	var report doctorReport
	if jerr := json.Unmarshal([]byte(stdout), &report); jerr != nil {
		t.Fatalf("doctor still prints JSON: %v", jerr)
	}
	if report.TokenValid {
		t.Error("tokenValid = true, want false")
	}
	if !strings.Contains(stderr, "auth login") {
		t.Errorf("stderr missing hint: %q", stderr)
	}
}

func TestAuthStatusJSON(t *testing.T) {
	cfg, _ := testConfig(t, true)
	stdout, _, err := run(t, "--config", cfg, "auth", "status", "--json")
	if err != nil {
		t.Fatalf("auth status: %v", err)
	}
	var report authStatusReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("auth status JSON: %v", err)
	}
	if !report.Present || !report.Valid {
		t.Errorf("present/valid = %v/%v, want true/true", report.Present, report.Valid)
	}
}

func TestAuthLogout(t *testing.T) {
	cfg, _ := testConfig(t, true)
	if _, _, err := run(t, "--config", cfg, "auth", "logout"); err != nil {
		t.Fatalf("auth logout: %v", err)
	}
	// A subsequent status should report no token.
	stdout, _, err := run(t, "--config", cfg, "auth", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report authStatusReport
	_ = json.Unmarshal([]byte(stdout), &report)
	if report.Present {
		t.Error("token still present after logout")
	}
}
