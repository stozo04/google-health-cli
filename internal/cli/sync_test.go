package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stozo04/google-health-cli/internal/config"
	"github.com/stozo04/google-health-cli/internal/dailylog"
)

func syncTestConfig(dailyLog string) *config.Config {
	return &config.Config{
		EllipticalTypes: []string{"ELLIPTICAL"},
		Zone2Low:        110,
		Zone2High:       130,
		DailyLog:        dailyLog,
	}
}

// loadInputDoc copies the committed input fixture to a temp file and loads it.
func loadInputDoc(t *testing.T) (*dailylog.Doc, string) {
	t.Helper()
	in, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "daily_log_input.json"))
	if err != nil {
		t.Fatalf("read input fixture: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "DAILY_LOG.json")
	if err := os.WriteFile(tmp, in, 0o644); err != nil {
		t.Fatalf("write temp daily log: %v", err)
	}
	doc, err := dailylog.Load(tmp)
	if err != nil {
		t.Fatalf("load doc: %v", err)
	}
	return doc, tmp
}

func TestDailyLogWriteGolden(t *testing.T) {
	doc, _ := loadInputDoc(t)
	cfg := syncTestConfig("")
	target := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	byDay, keys, dropped := bucketCardio(fixturePoints(t), target, 30, cfg.EllipticalTypes)
	if dropped != 5 {
		t.Errorf("dropped = %d, want 5 (2 CARDIO_WORKOUT + 3 STRENGTH_TRAINING)", dropped)
	}

	results, wroteAny, err := applySync(doc, byDay, keys, false, cfg)
	if err != nil {
		t.Fatalf("applySync: %v", err)
	}
	if !wroteAny {
		t.Error("wroteAny = false, want true")
	}

	wantStatus := map[string]string{
		"2026-05-18": "created",  // new day inserted
		"2026-05-19": "created",  // training appended to a day that lacked it
		"2026-06-02": "updated",  // prior ghealth training overwritten in place
		"2026-06-16": "conflict", // manual "Push" left untouched
	}
	got := map[string]string{}
	for _, r := range results {
		got[r.Date] = r.Status
	}
	for date, want := range wantStatus {
		if got[date] != want {
			t.Errorf("status[%s] = %q, want %q", date, got[date], want)
		}
	}

	out, err := doc.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	assertGolden(t, "daily_log_output.golden", out)
}

func TestSyncHumanGolden(t *testing.T) {
	doc, _ := loadInputDoc(t)
	cfg := syncTestConfig("")
	target := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	byDay, keys, dropped := bucketCardio(fixturePoints(t), target, 30, cfg.EllipticalTypes)
	results, _, err := applySync(doc, byDay, keys, false, cfg)
	if err != nil {
		t.Fatalf("applySync: %v", err)
	}
	summary := syncSummary{
		Target:           target.Format("2006-01-02"),
		Days:             30,
		DryRun:           false,
		DroppedNonCardio: dropped,
		Results:          results,
	}
	var buf bytes.Buffer
	if err := writeSyncHuman(&buf, summary, target, 30, dropped, false); err != nil {
		t.Fatalf("writeSyncHuman: %v", err)
	}
	assertGolden(t, "sync_human.golden", buf.Bytes())
}

// TestDailyLogSavePreservesCRLF proves the drop-in keeps the file's existing
// line-ending style (the live DAILY_LOG.json is CRLF) instead of churning it.
func TestDailyLogSavePreservesCRLF(t *testing.T) {
	in, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "daily_log_input.json"))
	if err != nil {
		t.Fatal(err)
	}
	crlf := bytes.ReplaceAll(bytes.ReplaceAll(in, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
	tmp := filepath.Join(t.TempDir(), "DAILY_LOG.json")
	if err := os.WriteFile(tmp, crlf, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := dailylog.Load(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(tmp); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("\r\n")) {
		t.Error("Save dropped CRLF line endings on a CRLF source")
	}
	// Every newline must be CRLF (no lone LF left behind).
	if bytes.Contains(bytes.ReplaceAll(out, []byte("\r\n"), nil), []byte("\n")) {
		t.Error("Save left a bare LF in a CRLF document")
	}
}

func TestDailyLogSaveKeepsLF(t *testing.T) {
	doc, tmp := loadInputDoc(t) // input fixture is LF
	if err := doc.Save(tmp); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("\r")) {
		t.Error("Save introduced CR into an LF document")
	}
}
