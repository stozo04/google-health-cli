package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	pts, err := c.ListExerciseDataPoints(context.Background(),
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC))
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
	rows := buildSessionRows(fixturePoints(t), []string{"ELLIPTICAL"})
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
