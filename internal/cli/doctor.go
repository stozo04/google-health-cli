package cli

import (
	"github.com/spf13/cobra"

	"github.com/stozo04/google-health-cli/internal/auth"
	"github.com/stozo04/google-health-cli/internal/version"
)

// doctorReport is the doctor JSON shape (GOAL.md §10). The old Python doctor was
// a passthrough of `ghealth doctor`; this is our own shape but keeps the
// tokenValid boolean so existing checks keep working. Key order is frozen.
type doctorReport struct {
	TokenValid bool     `json:"tokenValid"`
	BaseURL    string   `json:"baseURL"`
	User       string   `json:"user"`
	TokenPath  string   `json:"tokenPath"`
	ConfigPath string   `json:"configPath"`
	DailyLog   string   `json:"dailyLog"`
	Scopes     []string `json:"scopes"`
	Version    string   `json:"version"`
}

// newDoctorCmd implements `doctor` (GOAL.md §6, §10): print config + token
// validity as indent-2 JSON; exit 2 (with a stderr hint) when not authenticated.
func newDoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config + token validity",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := app.resolveConfig()
			if err != nil {
				return err
			}

			// Confirm a usable token can be obtained (refresh if needed),
			// without requiring daily_log — doctor is a diagnostic and should
			// report token state even before the log path is wired (GOAL.md §8).
			tokenValid := false
			if tok, lerr := auth.LoadToken(cfg.TokenCache); lerr != nil {
				app.logger.Warn("token cache unreadable", "path", cfg.TokenCache, "err", lerr)
			} else if tok != nil {
				client := app.buildAuthClient(cmd.Context(), cfg, tok)
				if _, terr := client.CurrentToken(); terr != nil {
					app.logger.Debug("token check failed", "err", terr)
				} else {
					tokenValid = true
				}
			}

			report := doctorReport{
				TokenValid: tokenValid,
				BaseURL:    cfg.BaseURL,
				User:       cfg.User,
				TokenPath:  cfg.TokenCache,
				ConfigPath: cfg.ConfigPath,
				DailyLog:   cfg.DailyLog,
				Scopes:     cfg.Scopes,
				Version:    version.Info().Version,
			}
			if err := writeJSON(cmd.OutOrStdout(), report); err != nil {
				return err
			}

			if !tokenValid {
				fprintln(cmd.ErrOrStderr(), "\nNot authenticated yet. Run:  google-health-cli auth login")
				return silentExit(ExitAuth)
			}
			return nil
		},
	}
}
