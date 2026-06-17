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
// dir, returning the config path.
func testConfig(t *testing.T, withToken bool) string {
	t.Helper()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	cfgPath := filepath.Join(dir, "config.json")

	cfg := map[string]any{
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
	return cfgPath
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
	cfg := testConfig(t, true)

	stdout, _, err := run(t, "--config", cfg, "sessions", "--json", "--date", "2026-06-16", "--days", "60")
	if err != nil {
		t.Fatalf("sessions: %v", err)
	}
	assertGolden(t, "sessions_json.golden", []byte(stdout))
}

func TestSessionsRawCommand(t *testing.T) {
	srv := fixtureServer(t)
	withBaseURL(t, srv.URL)
	cfg := testConfig(t, true)

	stdout, _, err := run(t, "--config", cfg, "sessions", "--raw", "--date", "2026-06-16", "--days", "60")
	if err != nil {
		t.Fatalf("sessions --raw: %v", err)
	}
	assertGolden(t, "sessions_raw.golden", []byte(stdout))
}

func TestSessionsNotAuthenticated(t *testing.T) {
	cfg := testConfig(t, false) // no token
	_, _, err := run(t, "--config", cfg, "sessions")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitAuth {
		t.Fatalf("err = %v, want ExitError code 2", err)
	}
}

func TestDataListCommand(t *testing.T) {
	srv := fixtureServer(t)
	withBaseURL(t, srv.URL)
	cfg := testConfig(t, true)

	// The fixture server returns the exercise payload for any path; `data list
	// exercise` should emit a JSON array on stdout and a count on stderr.
	stdout, stderr, err := run(t, "--config", cfg, "data", "list", "exercise", "--date", "2026-06-16", "--days", "60")
	if err != nil {
		t.Fatalf("data list: %v", err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &arr); err != nil {
		t.Fatalf("data list stdout not a JSON array: %v\n%s", err, stdout)
	}
	if len(arr) == 0 {
		t.Error("expected data points from the fixture")
	}
	if !strings.Contains(stderr, "exercise data point(s)") {
		t.Errorf("stderr missing count hint: %q", stderr)
	}
}

func TestDataListUnknownTypeIsUsageError(t *testing.T) {
	cfg := testConfig(t, true)
	_, _, err := run(t, "--config", cfg, "data", "list", "not-a-type")
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != ExitUsage {
		t.Fatalf("err = %v, want ExitError code %d", err, ExitUsage)
	}
}

func TestTypesListJSON(t *testing.T) {
	stdout, _, err := run(t, "types", "list", "--json")
	if err != nil {
		t.Fatalf("types list: %v", err)
	}
	var views []map[string]any
	if err := json.Unmarshal([]byte(stdout), &views); err != nil {
		t.Fatalf("types list --json not valid JSON: %v", err)
	}
	if len(views) != 31 {
		t.Errorf("types list returned %d, want 31", len(views))
	}
}

func TestDoctorValidToken(t *testing.T) {
	cfg := testConfig(t, true)
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
	cfg := testConfig(t, false)
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
	cfg := testConfig(t, true)
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
	cfg := testConfig(t, true)
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
