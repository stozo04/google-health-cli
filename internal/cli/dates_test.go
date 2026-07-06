package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/stozo04/google-health-cli/internal/api"
)

// TestResolveDateAnchorsToLocalZone: the --date/--days window must cover the
// user's LOCAL calendar days for every record family. Civil- and date-filtered
// types only ever read the wall-clock Y/M/D, but sample (physical_time) types
// convert the window bounds to RFC3339 instants — so the civil midnights
// resolveDate produces must carry the local zone. A UTC-anchored midnight
// shifts a heart-rate "day" by the UTC offset (~6h for US Central).
func TestResolveDateAnchorsToLocalZone(t *testing.T) {
	for _, in := range []string{"2026-06-16", "today", "yesterday", ""} {
		got, err := resolveDate(in)
		if err != nil {
			t.Fatalf("resolveDate(%q): %v", in, err)
		}
		if got.Location() != time.Local {
			t.Errorf("resolveDate(%q).Location() = %v, want time.Local (UTC-anchored midnights skew sample-type windows)",
				in, got.Location())
		}
	}
}

// TestSampleTypeWindowCoversLocalDay pins the end-to-end semantics with an
// explicit zone: one civil day at UTC-6 must reach a physical_time filter as
// the local midnights converted to instants — 06:00Z to 06:00Z — covering the
// user's day, not the UTC calendar day.
func TestSampleTypeWindowCoversLocalDay(t *testing.T) {
	zone := time.FixedZone("UTC-6", -6*60*60)
	target := time.Date(2026, 6, 16, 0, 0, 0, 0, zone)
	start, end := window(target, 1)

	hr, ok := api.LookupDataType("heart-rate")
	if !ok {
		t.Fatal("heart-rate missing from catalog")
	}
	filter := hr.RangeFilter(start, end)
	for _, bound := range []string{`"2026-06-16T06:00:00Z"`, `"2026-06-17T06:00:00Z"`} {
		if !strings.Contains(filter, bound) {
			t.Errorf("filter %q missing local-midnight instant %s", filter, bound)
		}
	}
}

// TestCivilTypeWindowUsesWallClock: for civil-filtered types the zone must stay
// invisible — the filter carries the bare wall-clock regardless of location, so
// anchoring resolveDate to time.Local cannot change any civil-type window.
func TestCivilTypeWindowUsesWallClock(t *testing.T) {
	zone := time.FixedZone("UTC-6", -6*60*60)
	target := time.Date(2026, 6, 16, 0, 0, 0, 0, zone)
	start, end := window(target, 2)

	ex, ok := api.LookupDataType("exercise")
	if !ok {
		t.Fatal("exercise missing from catalog")
	}
	filter := ex.RangeFilter(start, end)
	for _, bound := range []string{`"2026-06-15T00:00:00"`, `"2026-06-17T00:00:00"`} {
		if !strings.Contains(filter, bound) {
			t.Errorf("filter %q missing wall-clock bound %s", filter, bound)
		}
	}
}
