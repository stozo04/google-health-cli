package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stozo04/google-health-cli/internal/api"
)

// fixturePoints serves the committed exercise fixture over httptest and decodes
// it through the real api.Client, so the golden tests exercise the same decode
// path production uses.
func fixturePoints(t *testing.T) []api.DataPoint {
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

	c := api.New(srv.Client(), srv.URL, "users/me", nil)
	exercise, _ := api.LookupDataType("exercise")
	pts, err := c.ListDataPoints(context.Background(), exercise,
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC), true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return pts
}

// normalizeLF strips carriage returns so Windows checkouts compare byte-for-byte
// against the LF goldens (GOAL.md §15).
func normalizeLF(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r"), nil)
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "golden", name)
	// UPDATE_GOLDEN=1 rewrites the golden from the current output (goldens are
	// stored LF). Used to (re)generate goldens the Python oracle can't produce.
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, normalizeLF(got), 0o644); err != nil {
			t.Fatalf("update golden %s: %v", name, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	g, w := normalizeLF(got), normalizeLF(want)
	if !bytes.Equal(g, w) {
		t.Errorf("output does not match golden %s\n--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s",
			name, len(g), g, len(w), w)
	}
}

func TestSessionsJSONGolden(t *testing.T) {
	rows, _ := buildSessionRows(fixturePoints(t))
	var buf bytes.Buffer
	if err := writeJSON(&buf, rows); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	assertGolden(t, "sessions_json.golden", buf.Bytes())
}

func TestSessionsRawGolden(t *testing.T) {
	points := fixturePoints(t)
	raws := make([]json.RawMessage, 0, len(points))
	for _, p := range points {
		raws = append(raws, p.Raw)
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, raws); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	assertGolden(t, "sessions_raw.golden", buf.Bytes())
}

func TestBuildSessionRowsCountsPointsWithoutStart(t *testing.T) {
	points := append(fixturePoints(t), api.DataPoint{}, api.DataPoint{})
	rows, unparsed := buildSessionRows(points)
	if unparsed != 2 {
		t.Errorf("unparsed = %d, want 2", unparsed)
	}
	if len(rows) != len(points)-2 {
		t.Errorf("rows = %d, want %d", len(rows), len(points)-2)
	}
}

func TestReportUnparsedSessionsWarnsOnStderrOnly(t *testing.T) {
	var stderr bytes.Buffer
	reportUnparsedSessions(&stderr, 0)
	if stderr.Len() != 0 {
		t.Errorf("no unparsed points should print nothing, got %q", stderr.String())
	}
	reportUnparsedSessions(&stderr, 3)
	if !strings.Contains(stderr.String(), "3 exercise point(s)") || !strings.Contains(stderr.String(), "sessions --raw") {
		t.Errorf("warning = %q, want the count and the --raw pointer", stderr.String())
	}
}
