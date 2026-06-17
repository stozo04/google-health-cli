package cli

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/stozo04/google-health-cli/internal/api"
	"github.com/stozo04/google-health-cli/internal/health"
)

// sessionRow is one row of `sessions` output. The field/JSON-key order is frozen
// to match the Python dict (cli.py:cmd_sessions) byte-for-byte (GOAL.md §10):
// {date, exercise_type, elliptical, duration_min, avg_hr, calories, platform}.
type sessionRow struct {
	Date         string `json:"date"`
	ExerciseType string `json:"exercise_type"`
	Elliptical   bool   `json:"elliptical"`
	DurationMin  *int   `json:"duration_min"`
	AvgHR        any    `json:"avg_hr"`
	Calories     any    `json:"calories"`
	Platform     string `json:"platform"`
}

// newSessionsCmd ports cli.py:cmd_sessions (GOAL.md §10). It lists all exercise
// sessions; `*` marks cardio in human mode; --raw dumps the API JSON; --json
// emits the frozen row array.
func newSessionsCmd(app *App) *cobra.Command {
	var (
		date   string
		days   int
		raw    bool
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "sessions",
		Short: "List recent exercise sessions (all types)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runSessions(cmd, date, days, raw, asJSON)
		},
	}
	cmd.Flags().StringVar(&date, "date", "today", "today | yesterday | YYYY-MM-DD")
	cmd.Flags().IntVar(&days, "days", 7, "number of days back to include")
	cmd.Flags().BoolVar(&raw, "raw", false, "dump the raw API JSON data points")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func (a *App) runSessions(cmd *cobra.Command, date string, days int, raw, asJSON bool) error {
	target, err := resolveDate(date)
	if err != nil {
		return withCode(ExitUsage, err)
	}
	points, err := a.listSessions(cmd.Context(), target, days)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if raw {
		// Dump the raw API data-point list exactly (cli.py: json.dumps(points,
		// indent=2)). RawMessage preserves each point's bytes/key order.
		rawPoints := make([]json.RawMessage, 0, len(points))
		for _, p := range points {
			rawPoints = append(rawPoints, p.Raw)
		}
		return writeJSON(out, rawPoints)
	}

	rows := buildSessionRows(points, a.cfg.EllipticalTypes)

	if asJSON {
		return writeJSON(out, rows)
	}
	if len(rows) == 0 {
		fprintf(out, "No exercise sessions found in the last %d day(s).\n", days)
		return nil
	}
	fprintf(out, "%d session(s) in the last %d day(s) (* = counts as cardio, others ignored):\n\n",
		len(rows), days)
	for _, r := range rows {
		mark := " "
		if r.Elliptical {
			mark = "*"
		}
		hr := "no HR"
		if health.Truthy(r.AvgHR) {
			hr = health.Render(r.AvgHR) + " avg"
		}
		dur := "?"
		if r.DurationMin != nil && *r.DurationMin != 0 {
			dur = strconv.Itoa(*r.DurationMin)
		}
		fprintf(out, "  %s %s  %-18s %3s min  %7s  %s\n",
			mark, r.Date, r.ExerciseType, dur, hr, r.Platform)
	}
	return nil
}

// listSessions resolves the window and fetches the raw points for it.
func (a *App) listSessions(ctx context.Context, target time.Time, days int) ([]api.DataPoint, error) {
	client, _, err := a.apiClient(ctx)
	if err != nil {
		return nil, err
	}
	start, end := window(target, days)
	points, err := client.ListExerciseDataPoints(ctx, start, end)
	if err != nil {
		return nil, withCode(ExitAuth, err)
	}
	return points, nil
}

// buildSessionRows parses, drops rows without a start, sorts by start ascending,
// and maps to the frozen row shape (cli.py:cmd_sessions).
func buildSessionRows(points []api.DataPoint, ellipticalTypes []string) []sessionRow {
	parsed := make([]health.Session, 0, len(points))
	for _, p := range points {
		s := health.ParseSession(p)
		if s.Start == nil {
			continue
		}
		parsed = append(parsed, s)
	}
	sort.SliceStable(parsed, func(i, j int) bool {
		return parsed[i].Start.Before(*parsed[j].Start)
	})

	rows := make([]sessionRow, 0, len(parsed))
	for _, s := range parsed {
		rows = append(rows, sessionRow{
			Date:         s.Start.Format("2006-01-02"),
			ExerciseType: s.ExerciseType,
			Elliptical:   health.IsElliptical(s, ellipticalTypes),
			DurationMin:  s.DurationMin,
			AvgHR:        s.AvgHR,
			Calories:     s.Calories,
			Platform:     s.Platform,
		})
	}
	return rows
}
