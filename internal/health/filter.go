package health

import "strings"

// IsElliptical is the ALLOWLIST dedup guard (health.py:is_elliptical). It returns
// true only when the session's exercise_type is in the configured allowlist
// (case-insensitive). Anything else — STRENGTH_TRAINING, CARDIO_WORKOUT,
// WALKING, unknown, or empty — returns false and is ignored, so the strength
// work speediance-cli already logs is never double-counted (GOAL.md §1).
func IsElliptical(s Session, ellipticalTypes []string) bool {
	if s.ExerciseType == "" {
		return false
	}
	want := strings.ToUpper(s.ExerciseType)
	for _, t := range ellipticalTypes {
		if strings.ToUpper(t) == want {
			return true
		}
	}
	return false
}
