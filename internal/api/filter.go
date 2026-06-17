// Package api is the Google Health API client: the single place that owns the
// exercise dataPoints.list request URL, query params, filter syntax, and
// response decode (GOAL.md §9). The Python tool never spoke this wire (it shelled
// out to `ghealth`); the syntax here is built from scratch against the verified
// ghealth behavior and Steven's real Pixel Watch data.
package api

import (
	"fmt"
	"time"
)

// civilLayout formats a naive local (civil) wall-clock time with NO trailing Z
// and NO offset. The exercise endpoint filters on
// exercise.interval.civil_start_time; a UTC 'Z' is rejected with
// INVALID_DATA_POINT_FILTER_CIVIL_DATE_TIME_FORMAT (GOAL.md §9, health.py _civil).
const civilLayout = "2006-01-02T15:04:05"

// Civil renders t as a civil date-time string. Only the wall-clock fields are
// used, so the time.Time's location is irrelevant to the output.
func Civil(t time.Time) string {
	return t.Format(civilLayout)
}

// FilterFromRange builds the frozen filter string for a [from, to) civil window
// (GOAL.md §9): >= inclusive lower bound, < exclusive upper bound, joined by
// " AND " with double-quoted civil values.
//
//	exercise.interval.civil_start_time >= "<FROM>" AND exercise.interval.civil_start_time < "<TO>"
func FilterFromRange(from, to time.Time) string {
	return fmt.Sprintf(
		`exercise.interval.civil_start_time >= %q AND exercise.interval.civil_start_time < %q`,
		Civil(from), Civil(to),
	)
}
