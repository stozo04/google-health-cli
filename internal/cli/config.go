package cli

import (
	"github.com/spf13/cobra"
)

// configView is the stable shape emitted by `config show --json`. client_secret
// is always blanked here — this view is a convenience, never a secret dump
// (GOAL.md §6, §7).
type configView struct {
	DailyLog        string   `json:"daily_log"`
	EllipticalTypes []string `json:"elliptical_types"`
	Zone2Low        int      `json:"zone2_low"`
	Zone2High       int      `json:"zone2_high"`
	ClientID        string   `json:"client_id"`
	ClientSecret    string   `json:"client_secret"`
	BaseURL         string   `json:"base_url"`
	User            string   `json:"user"`
	TokenCache      string   `json:"token_cache"`
	Scopes          []string `json:"scopes"`
	ConfigPath      string   `json:"config_path"`
}

// newConfigCmd implements `config [show|path]` (GOAL.md §6), a convenience for
// inspecting the resolved configuration without hand-reading config.json.
func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect configuration (show/path)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigShowCmd(app), newConfigPathCmd(app))
	return cmd
}

func newConfigShowCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the resolved effective config (client_secret redacted)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := app.resolveConfig()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				view := configView{
					DailyLog:        cfg.DailyLog,
					EllipticalTypes: cfg.EllipticalTypes,
					Zone2Low:        cfg.Zone2Low,
					Zone2High:       cfg.Zone2High,
					ClientID:        cfg.ClientID,
					ClientSecret:    "", // never emit the secret in JSON.
					BaseURL:         cfg.BaseURL,
					User:            cfg.User,
					TokenCache:      cfg.TokenCache,
					Scopes:          cfg.Scopes,
					ConfigPath:      cfg.ConfigPath,
				}
				return writeJSON(out, view)
			}
			secret := ""
			if cfg.ClientSecret != "" {
				secret = "****"
			}
			fprintf(out, "daily_log:        %s\n", cfg.DailyLog)
			fprintf(out, "elliptical_types: %v\n", cfg.EllipticalTypes)
			fprintf(out, "zone2_low:        %d\n", cfg.Zone2Low)
			fprintf(out, "zone2_high:       %d\n", cfg.Zone2High)
			fprintf(out, "client_id:        %s\n", cfg.ClientID)
			fprintf(out, "client_secret:    %s\n", secret)
			fprintf(out, "base_url:         %s\n", cfg.BaseURL)
			fprintf(out, "user:             %s\n", cfg.User)
			fprintf(out, "token_cache:      %s\n", cfg.TokenCache)
			fprintf(out, "scopes:           %v\n", cfg.Scopes)
			fprintf(out, "config_path:      %s\n", cfg.ConfigPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newConfigPathCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config.json path in use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := app.resolveConfig()
			if err != nil {
				return err
			}
			fprintln(cmd.OutOrStdout(), cfg.ConfigPath)
			return nil
		},
	}
}
