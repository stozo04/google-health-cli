package api

import (
	"testing"
	"time"
)

func TestFilterFromRange(t *testing.T) {
	from := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	got := FilterFromRange(from, to)
	want := `exercise.interval.civil_start_time >= "2026-06-14T00:00:00" AND exercise.interval.civil_start_time < "2026-06-18T00:00:00"`
	if got != want {
		t.Errorf("FilterFromRange:\n got: %s\nwant: %s", got, want)
	}
}

func TestCivilHasNoZone(t *testing.T) {
	// The endpoint rejects a trailing Z / offset; Civil must emit bare wall-clock.
	tm := time.Date(2026, 6, 16, 20, 33, 51, 0, time.FixedZone("CST", -5*3600))
	if got := Civil(tm); got != "2026-06-16T20:33:51" {
		t.Errorf("Civil = %q, want 2026-06-16T20:33:51 (no zone)", got)
	}
}
