package cli

import (
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/stozo04/google-health-cli/internal/api"
	"github.com/stozo04/google-health-cli/internal/config"
	"github.com/stozo04/google-health-cli/internal/dailylog"
	"github.com/stozo04/google-health-cli/internal/health"
)

// syncResult is one day's outcome in the `sync --json` summary. Key order frozen
// (GOAL.md §10).
type syncResult struct {
	Date        string `json:"date"`
	DurationMin any    `json:"duration_min"`
	AvgHR       any    `json:"avg_hr"`
	Calories    any    `json:"calories"`
	Zone2       string `json:"zone2"`
	Status      string `json:"status"`
}

// syncSummary is the `sync --json` payload (GOAL.md §10). Key order frozen.
type syncSummary struct {
	Target           string       `json:"target"`
	Days             int          `json:"days"`
	DryRun           bool         `json:"dry_run"`
	DroppedNonCardio int          `json:"dropped_non_cardio"`
	Results          []syncResult `json:"results"`
}

// newSyncCmd ports cli.py:cmd_sync (GOAL.md §10): pull recent sessions, keep
// elliptical only, and upsert each day into DAILY_LOG.json. Adds the new opt-in
// --json summary.
func newSyncCmd(app *App) *cobra.Command {
	var (
		date   string
		days   int
		dryRun bool
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Write elliptical sessions into DAILY_LOG.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runSync(cmd, date, days, dryRun, asJSON)
		},
	}
	cmd.Flags().StringVar(&date, "date", "today", "today | yesterday | YYYY-MM-DD")
	cmd.Flags().IntVar(&days, "days", 3, "number of days back to sync")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be written without writing")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable summary (stdout JSON only)")
	return cmd
}

func (a *App) runSync(cmd *cobra.Command, date string, days int, dryRun, asJSON bool) error {
	target, err := resolveDate(date)
	if err != nil {
		return withCode(ExitUsage, err)
	}
	points, err := a.listSessions(cmd.Context(), target, days)
	if err != nil {
		return err
	}

	byDay, dayKeys, dropped := bucketCardio(points, target, days, a.cfg.EllipticalTypes)

	summary := syncSummary{
		Target:           target.Format("2006-01-02"),
		Days:             days,
		DryRun:           dryRun,
		DroppedNonCardio: dropped,
		Results:          make([]syncResult, 0, len(dayKeys)),
	}

	var doc *dailylog.Doc
	if len(dayKeys) > 0 {
		doc, err = dailylog.Load(a.cfg.DailyLog)
		if err != nil {
			return withCode(ExitConfig, err)
		}
		results, wroteAny, err := applySync(doc, byDay, dayKeys, dryRun, a.cfg)
		if err != nil {
			return withCode(ExitFailure, err)
		}
		summary.Results = results
		if !dryRun && wroteAny {
			if err := doc.Save(a.cfg.DailyLog); err != nil {
				return withCode(ExitFailure, err)
			}
		}
	}

	out := cmd.OutOrStdout()
	if asJSON {
		return writeJSON(out, summary)
	}
	return writeSyncHuman(out, summary, target, days, dropped, dryRun)
}

// bucketCardio parses every point, keeps elliptical-only sessions with a start,
// buckets them by local date, restricts to the requested window, and returns the
// buckets, the sorted day keys, and the count of dropped non-cardio sessions
// (cli.py:cmd_sync). The dropped count is computed before the window filter, so
// it counts every non-elliptical/start-less session regardless of date.
func bucketCardio(points []api.DataPoint, target time.Time, days int, ellipticalTypes []string) (map[string][]health.Session, []string, int) {
	all := make([]health.Session, 0, len(points))
	for _, p := range points {
		all = append(all, health.ParseSession(p))
	}

	cardio := make([]health.Session, 0, len(all))
	for _, s := range all {
		if s.Start != nil && health.IsElliptical(s, ellipticalTypes) {
			cardio = append(cardio, s)
		}
	}
	dropped := len(all) - len(cardio)

	byDay := map[string][]health.Session{}
	for _, s := range cardio {
		d := s.Start.Format("2006-01-02")
		byDay[d] = append(byDay[d], s)
	}

	wanted := wantedDays(target, days)
	for d := range byDay {
		if !wanted[d] {
			delete(byDay, d)
		}
	}

	keys := make([]string, 0, len(byDay))
	for d := range byDay {
		keys = append(keys, d)
	}
	sort.Strings(keys)
	return byDay, keys, dropped
}

// applySync builds the cardio entry for each sorted day and upserts it (unless
// dry-run), returning the per-day results and whether anything was written
// (cli.py:cmd_sync loop).
func applySync(doc *dailylog.Doc, byDay map[string][]health.Session, dayKeys []string, dryRun bool, cfg *config.Config) ([]syncResult, bool, error) {
	results := make([]syncResult, 0, len(dayKeys))
	wroteAny := false
	for _, dayISO := range dayKeys {
		entry, ok := dailylog.BuildCardioEntry(byDay[dayISO], cfg.Zone2Low, cfg.Zone2High)
		if !ok {
			continue
		}
		status := "dry-run"
		if !dryRun {
			st, err := doc.Upsert(dayISO, entry.Object)
			if err != nil {
				return nil, false, err
			}
			status = st
			if st == "created" || st == "updated" {
				wroteAny = true
			}
		}
		results = append(results, syncResult{
			Date:        dayISO,
			DurationMin: entry.DurationMin,
			AvgHR:       entry.AvgHR,
			Calories:    entry.Calories,
			Zone2:       entry.Zone2,
			Status:      status,
		})
	}
	return results, wroteAny, nil
}

// writeSyncHuman reproduces the human stdout of cli.py:cmd_sync (lines 110-136).
func writeSyncHuman(out io.Writer, summary syncSummary, target time.Time, days, dropped int, dryRun bool) error {
	if len(summary.Results) == 0 {
		fprintf(out, "No elliptical sessions to log for the last %d day(s) ending %s. (%d non-cardio session(s) ignored.)\n",
			days, target.Format("2006-01-02"), dropped)
		return nil
	}
	tag := "synced"
	if dryRun {
		tag = "[dry-run] would write"
	}
	fprintf(out, "%s %d cardio day(s); %d non-cardio ignored.\n\n", tag, len(summary.Results), dropped)
	for _, r := range summary.Results {
		fprintf(out, "  %s: %s min, avg HR %s [%s], %s kcal%s\n",
			r.Date, pyNum(r.DurationMin), pyNum(r.AvgHR), r.Zone2, pyNum(r.Calories), syncFlag(r.Status))
	}
	return nil
}

// syncFlag maps a status to the trailing human marker (cli.py:cmd_sync FLAG).
func syncFlag(status string) string {
	switch status {
	case "created":
		return "  (new day)"
	case "updated":
		return "  (updated)"
	case "conflict":
		return "  !! SKIPPED - manual session already logged this day"
	default: // "dry-run"
		return ""
	}
}

// pyNum renders a numeric entry value the way Python's f-string would: "None"
// for nil, an int with no decimal point, a float with CPython's repr.
func pyNum(v any) string {
	if v == nil {
		return "None"
	}
	if s := health.Render(v); s != "" {
		return s
	}
	return "None"
}
