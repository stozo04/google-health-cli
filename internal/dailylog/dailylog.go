package dailylog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/stozo04/google-health-cli/internal/health"
)

// weekdays maps Python's date.weekday() (Mon=0 … Sun=6) to the short labels
// daily_log.py uses for a new day's "weekday" field.
var weekdays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// Doc is a loaded DAILY_LOG.json. The full document is held as an ordered Object
// (so unknown keys round-trip); days are additionally parsed into ordered
// objects because they are the only thing we mutate.
type Doc struct {
	obj  *Object
	days []*Object
	crlf bool // the source used CRLF line endings; preserve them on write.
}

// Load reads and parses a DAILY_LOG.json (daily_log.py:load).
func Load(path string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	obj := NewObject()
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	days := make([]*Object, 0)
	if raw, ok := obj.Get("days"); ok {
		if err := json.Unmarshal(raw, &days); err != nil {
			return nil, fmt.Errorf("parse days in %s: %w", path, err)
		}
	}
	return &Doc{obj: obj, days: days, crlf: bytes.Contains(data, []byte("\r\n"))}, nil
}

// Bytes serializes the document exactly as Python's
// json.dump(doc, ensure_ascii=False, indent=2) + a trailing newline (GOAL.md
// §11): the mutated days are written back into the ordered document, then the
// whole thing is indented with HTML escaping off and given one trailing '\n'.
func (d *Doc) Bytes() ([]byte, error) {
	daysRaw, err := rawJSON(d.days)
	if err != nil {
		return nil, err
	}
	d.obj.SetRaw("days", daysRaw)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d.obj); err != nil {
		return nil, err
	}
	out := append(bytes.TrimRight(buf.Bytes(), "\n"), '\n')
	return out, nil
}

// Save writes the document to path. DAILY_LOG.json is an external,
// world-readable user document (not a secret), so 0644 mirrors Python's default.
//
// Line endings are preserved: if the loaded file used CRLF (which is what
// Python's text-mode json.dump produces on Windows, and what the live file
// uses), the write uses CRLF too. This keeps the drop-in diff clean instead of
// rewriting every line ending (GOAL.md §2, §15). Bytes() stays the LF-canonical
// form the goldens are compared against.
func (d *Doc) Save(path string) error {
	out, err := d.Bytes()
	if err != nil {
		return err
	}
	if d.crlf {
		out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	return os.WriteFile(path, out, 0o644) //nolint:gosec // shared user artifact, not a secret.
}

// FindDay returns the day object with the given ISO date, or nil.
func (d *Doc) FindDay(dayISO string) *Object {
	for _, day := range d.days {
		if getString(day, "date") == dayISO {
			return day
		}
	}
	return nil
}

// Upsert writes entry into day[dayISO].training, creating the day if needed
// (daily_log.py:upsert_cardio). It returns "created", "updated", or "conflict".
// "conflict" means a non-ghealth training session is already logged that day; it
// is left untouched so a manual entry is never destroyed (GOAL.md §11).
func (d *Doc) Upsert(dayISO string, entry *Object) (string, error) {
	if day := d.FindDay(dayISO); day != nil {
		existingTruthy := false
		if raw, ok := day.Get("training"); ok {
			truthy, source := trainingState(raw)
			existingTruthy = truthy
			if truthy && source != "ghealth" {
				return "conflict", nil
			}
		}
		entryRaw, err := rawJSON(entry)
		if err != nil {
			return "", err
		}
		day.SetRaw("training", entryRaw)
		if existingTruthy {
			return "updated", nil
		}
		return "created", nil
	}

	newDay, err := d.newDaySkeleton(dayISO, entry)
	if err != nil {
		return "", err
	}
	d.insertDaySorted(dayISO, newDay)
	return "created", nil
}

// newDaySkeleton builds the exact new-day object daily_log.py:upsert_cardio
// inserts: date, weekday, partial, training, body, watch (key order frozen).
func (d *Doc) newDaySkeleton(dayISO string, entry *Object) (*Object, error) {
	wd, err := weekdayLabel(dayISO)
	if err != nil {
		return nil, err
	}
	entryRaw, err := rawJSON(entry)
	if err != nil {
		return nil, err
	}
	day := NewObject()
	if err := day.Set("date", dayISO); err != nil {
		return nil, err
	}
	if err := day.Set("weekday", wd); err != nil {
		return nil, err
	}
	if err := day.Set("partial", true); err != nil {
		return nil, err
	}
	day.SetRaw("training", entryRaw)
	day.SetRaw("body", json.RawMessage(`{"weight_lb":null,"waist_in":null}`))
	day.SetRaw("watch", json.RawMessage(`{"sleep_hrs":null,"resting_hr":null,"steps":null}`))
	return day, nil
}

// insertDaySorted inserts newDay keeping days sorted by ISO date, before the
// first day whose date string sorts after dayISO (daily_log.py:upsert_cardio).
func (d *Doc) insertDaySorted(dayISO string, newDay *Object) {
	idx := len(d.days)
	for i, day := range d.days {
		if getString(day, "date") > dayISO {
			idx = i
			break
		}
	}
	d.days = append(d.days, nil)
	copy(d.days[idx+1:], d.days[idx:])
	d.days[idx] = newDay
}

// trainingState reports Python truthiness of a training value and its "source".
// null / {} / absent are falsy (treated as "no training"); a non-empty object is
// truthy and its source decides the conflict guard.
func trainingState(raw json.RawMessage) (truthy bool, source string) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		// Not an object. null is falsy; any other non-null value is truthy with
		// an empty source, which the conflict guard treats as "leave it alone".
		t := bytes.TrimSpace(raw)
		if len(t) == 0 || string(t) == "null" {
			return false, ""
		}
		return true, ""
	}
	if len(obj) == 0 { // null -> nil map; {} -> empty map; both falsy.
		return false, ""
	}
	if s, ok := obj["source"]; ok {
		var src string
		_ = json.Unmarshal(s, &src)
		return true, src
	}
	return true, ""
}

// getString returns a string-valued field of an object, or "".
func getString(o *Object, key string) string {
	raw, ok := o.Get(key)
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// weekdayLabel maps an ISO date to Python's WEEKDAYS[date.weekday()].
func weekdayLabel(dayISO string) (string, error) {
	t, err := time.Parse("2006-01-02", dayISO)
	if err != nil {
		return "", fmt.Errorf("invalid date %q: %w", dayISO, err)
	}
	// Go: Sunday=0..Saturday=6; Python: Monday=0..Sunday=6.
	idx := (int(t.Weekday()) + 6) % 7
	return weekdays[idx], nil
}

// ---- Merge + entry building (daily_log.py) -----------------------------------

// summary is the merged result of one day's elliptical sessions.
type summary struct {
	DurationMin any // int or nil
	Calories    any // int or nil
	AvgHR       any // int (rounded), a passed-through value, or nil
	Count       int
}

// mergeSessions combines 1+ elliptical sessions into a single cardio summary
// (daily_log.py:merge_sessions): duration and calories sum; avg HR is
// duration-weighted; the operation order matches Python to avoid FP drift, and
// round() is round-half-to-even (math.RoundToEven). GOAL.md §11.
func mergeSessions(sessions []health.Session) *summary {
	if len(sessions) == 0 {
		return nil
	}

	totalMin := 0
	for _, s := range sessions {
		if s.DurationMin != nil { // "duration_min or 0"
			totalMin += *s.DurationMin
		}
	}

	totalCal := 0.0
	for _, s := range sessions {
		if f, ok := health.NumFloat(s.Calories); ok { // "calories or 0"
			totalCal += f
		}
	}

	weighted := 0.0
	weight := 0
	for _, s := range sessions {
		durTruthy := s.DurationMin != nil && *s.DurationMin != 0
		if health.Truthy(s.AvgHR) && durTruthy {
			hr, _ := health.NumFloat(s.AvgHR)
			weighted += hr * float64(*s.DurationMin)
			weight += *s.DurationMin
		}
	}

	var avgHR any
	switch {
	case weight != 0:
		avgHR = int(math.RoundToEven(weighted / float64(weight)))
	case len(sessions) == 1:
		avgHR = sessions[0].AvgHR // raw passthrough (int | pyFloat | nil).
	default:
		avgHR = nil
	}

	var durationMin any
	if totalMin != 0 { // "total_min or None"
		durationMin = totalMin
	}
	var calories any
	if totalCal != 0 { // "round(total_cal) if total_cal else None"
		calories = int(math.RoundToEven(totalCal))
	}

	return &summary{
		DurationMin: durationMin,
		Calories:    calories,
		AvgHR:       avgHR,
		Count:       len(sessions),
	}
}

// Zone2Label is the calm band readout (daily_log.py:zone2_label): "no HR" /
// "below band (<low)" / "above band (>high)" / "in band (low-high)".
func Zone2Label(avgHR any, low, high int) string {
	f, ok := health.NumFloat(avgHR)
	if !ok { // None -> no HR.
		return "no HR"
	}
	if f < float64(low) {
		return fmt.Sprintf("below band (<%d)", low)
	}
	if f > float64(high) {
		return fmt.Sprintf("above band (>%d)", high)
	}
	return fmt.Sprintf("in band (%d-%d)", low, high)
}

// CardioEntry is the data a caller needs from a built entry for human/JSON sync
// output, alongside the ordered Object written to disk.
type CardioEntry struct {
	Object      *Object
	DurationMin any
	AvgHR       any
	Calories    any
	Zone2       string
	Count       int
}

// BuildCardioEntry builds the training object for an elliptical cardio day
// (daily_log.py:build_cardio_entry). The session name is the zone-neutral
// "Elliptical" — the single intentional change from the Python's hardcoded
// "Zone 2 elliptical" (GOAL.md §11). Returns ok=false when there are no
// sessions. Field/JSON-key order is frozen.
func BuildCardioEntry(sessions []health.Session, zone2Low, zone2High int) (*CardioEntry, bool) {
	sum := mergeSessions(sessions)
	if sum == nil {
		return nil, false
	}
	zone2 := Zone2Label(sum.AvgHR, zone2Low, zone2High)

	e := NewObject()
	must(e.Set("session", "Elliptical"))
	must(e.Set("type", "cardio"))
	must(e.Set("completed", true))
	must(e.Set("source", "ghealth"))
	must(e.Set("duration_min", sum.DurationMin))
	must(e.Set("avg_hr", sum.AvgHR))
	must(e.Set("calories", sum.Calories))
	must(e.Set("zone2", zone2))
	if sum.Count > 1 {
		must(e.Set("sessions", sum.Count))
	}

	return &CardioEntry{
		Object:      e,
		DurationMin: sum.DurationMin,
		AvgHR:       sum.AvgHR,
		Calories:    sum.Calories,
		Zone2:       zone2,
		Count:       sum.Count,
	}, true
}

// must panics on an Object.Set error. Set only fails if a value can't be JSON
// marshaled; every value we pass (strings, ints, bools, pyFloat, nil) always
// can, so a failure is a programming error, not a runtime condition.
func must(err error) {
	if err != nil {
		panic(err)
	}
}
