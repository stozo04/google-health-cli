package dailylog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stozo04/google-health-cli/internal/health"
)

func intp(n int) *int { return &n }

// session mirrors a parsed ELLIPTICAL: 30 min, HR 122, 245 kcal.
func ellipticalSession() health.Session {
	return health.Session{ExerciseType: "ELLIPTICAL", DurationMin: intp(30), AvgHR: 122, Calories: 245}
}

func docFromJSON(t *testing.T, s string) (*Doc, string) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "DAILY_LOG.json")
	if err := os.WriteFile(tmp, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(tmp)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return doc, tmp
}

func TestZone2Label(t *testing.T) {
	cases := []struct {
		hr   any
		want string
	}{
		{122, "in band (110-130)"},
		{95, "below band (<110)"},
		{150, "above band (>130)"},
		{110, "in band (110-130)"}, // boundary inclusive
		{130, "in band (110-130)"}, // boundary inclusive
		{nil, "no HR"},
	}
	for _, c := range cases {
		if got := Zone2Label(c.hr, 110, 130); got != c.want {
			t.Errorf("Zone2Label(%v) = %q, want %q", c.hr, got, c.want)
		}
	}
}

func TestMergeMultipleSessions(t *testing.T) {
	m := mergeSessions([]health.Session{ellipticalSession(), ellipticalSession()})
	if m.DurationMin != 60 {
		t.Errorf("DurationMin = %v, want 60", m.DurationMin)
	}
	if m.Calories != 490 {
		t.Errorf("Calories = %v, want 490", m.Calories)
	}
	if m.Count != 2 {
		t.Errorf("Count = %d, want 2", m.Count)
	}
	if m.AvgHR != 122 { // duration-weighted, same HR
		t.Errorf("AvgHR = %v, want 122", m.AvgHR)
	}
}

func TestMergeDurationWeightedHR(t *testing.T) {
	// 20 min @ 100 + 40 min @ 130 = (2000 + 5200) / 60 = 120.0 -> 120.
	a := health.Session{DurationMin: intp(20), AvgHR: 100, Calories: 100}
	b := health.Session{DurationMin: intp(40), AvgHR: 130, Calories: 200}
	m := mergeSessions([]health.Session{a, b})
	if m.AvgHR != 120 {
		t.Errorf("weighted AvgHR = %v, want 120", m.AvgHR)
	}
	if m.DurationMin != 60 || m.Calories != 300 {
		t.Errorf("DurationMin/Calories = %v/%v, want 60/300", m.DurationMin, m.Calories)
	}
}

func TestMergeNoHRFallsBackToNil(t *testing.T) {
	// Multiple sessions, none with HR -> avg_hr None -> "no HR".
	a := health.Session{DurationMin: intp(30), AvgHR: nil, Calories: 100}
	b := health.Session{DurationMin: intp(30), AvgHR: nil, Calories: 100}
	m := mergeSessions([]health.Session{a, b})
	if m.AvgHR != nil {
		t.Errorf("AvgHR = %v, want nil", m.AvgHR)
	}
	if Zone2Label(m.AvgHR, 110, 130) != "no HR" {
		t.Errorf("zone2 = %q, want no HR", Zone2Label(m.AvgHR, 110, 130))
	}
}

func TestBuildCardioEntryNameIsElliptical(t *testing.T) {
	entry, ok := BuildCardioEntry([]health.Session{ellipticalSession()}, 110, 130)
	if !ok {
		t.Fatal("BuildCardioEntry ok=false")
	}
	raw, _ := entry.Object.Get("session")
	var name string
	_ = json.Unmarshal(raw, &name)
	if name != "Elliptical" {
		t.Errorf("session = %q, want Elliptical (zone-neutral, GOAL §11)", name)
	}
	src, _ := entry.Object.Get("source")
	if string(src) != `"ghealth"` {
		t.Errorf("source = %s, want \"ghealth\"", src)
	}
}

func TestUpsertCreatesThenUpdatesIdempotently(t *testing.T) {
	doc, _ := docFromJSON(t, `{"days":[{"date":"2026-06-16","weekday":"Tue","nutrition":{"calories_food":1500}}]}`)
	entry, _ := BuildCardioEntry([]health.Session{ellipticalSession()}, 110, 130)

	status, err := doc.Upsert("2026-06-16", entry.Object)
	if err != nil {
		t.Fatal(err)
	}
	if status != "created" {
		t.Errorf("first upsert status = %q, want created", status)
	}
	day := doc.FindDay("2026-06-16")
	traw, _ := day.Get("training")
	var tr struct {
		Type  string `json:"type"`
		AvgHR int    `json:"avg_hr"`
	}
	_ = json.Unmarshal(traw, &tr)
	if tr.Type != "cardio" || tr.AvgHR != 122 {
		t.Errorf("training = %s, want type cardio avg_hr 122", traw)
	}
	// Nutrition is untouched.
	if !strings.Contains(string(mustBytes(t, doc)), `"calories_food": 1500`) {
		t.Error("nutrition was modified")
	}

	// Re-sync overwrites our own entry, not a duplicate.
	status2, _ := doc.Upsert("2026-06-16", entry.Object)
	if status2 != "updated" {
		t.Errorf("second upsert status = %q, want updated", status2)
	}
}

func TestUpsertNeverClobbersManual(t *testing.T) {
	doc, _ := docFromJSON(t, `{"days":[{"date":"2026-06-16","weekday":"Tue","training":{"session":"Push","source":"manual"}}]}`)
	entry, _ := BuildCardioEntry([]health.Session{ellipticalSession()}, 110, 130)
	status, _ := doc.Upsert("2026-06-16", entry.Object)
	if status != "conflict" {
		t.Errorf("status = %q, want conflict", status)
	}
	day := doc.FindDay("2026-06-16")
	if getString(probeTraining(t, day), "session") != "Push" {
		t.Error("manual training was clobbered")
	}
}

func TestUpsertCreatesNewDayInOrder(t *testing.T) {
	doc, _ := docFromJSON(t, `{"days":[{"date":"2026-06-14"},{"date":"2026-06-18"}]}`)
	entry, _ := BuildCardioEntry([]health.Session{ellipticalSession()}, 110, 130)
	if _, err := doc.Upsert("2026-06-16", entry.Object); err != nil {
		t.Fatal(err)
	}
	var dates []string
	for _, d := range doc.days {
		dates = append(dates, getString(d, "date"))
	}
	want := []string{"2026-06-14", "2026-06-16", "2026-06-18"}
	if strings.Join(dates, ",") != strings.Join(want, ",") {
		t.Errorf("day order = %v, want %v", dates, want)
	}
}

func TestUpsertAppendsTrainingKeyAtEnd(t *testing.T) {
	// A day with no training: the training key must be appended last (GOAL §11).
	doc, _ := docFromJSON(t, `{"days":[{"date":"2026-06-16","weekday":"Tue","watch":{"steps":null}}]}`)
	entry, _ := BuildCardioEntry([]health.Session{ellipticalSession()}, 110, 130)
	if _, err := doc.Upsert("2026-06-16", entry.Object); err != nil {
		t.Fatal(err)
	}
	day := doc.FindDay("2026-06-16")
	keys := day.Keys()
	if keys[len(keys)-1] != "training" {
		t.Errorf("last key = %q, want training (appended at end); keys=%v", keys[len(keys)-1], keys)
	}
}

func mustBytes(t *testing.T, d *Doc) []byte {
	t.Helper()
	b, err := d.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// probeTraining returns a day's training value parsed as an ordered Object.
func probeTraining(t *testing.T, day *Object) *Object {
	t.Helper()
	raw, ok := day.Get("training")
	if !ok {
		t.Fatal("no training")
	}
	o := NewObject()
	if err := json.Unmarshal(raw, o); err != nil {
		t.Fatal(err)
	}
	return o
}
