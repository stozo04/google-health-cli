package cli

import (
	"encoding/json"

	"github.com/spf13/cobra"
)

// newAPICmd is the raw, read-only escape hatch: `api get <path>` issues an
// authenticated GET to any v4 path and prints the response. It reaches endpoints
// the ergonomic commands don't model (users profile/settings/identity, a single
// dataPoint by resource name, reconcile, …). Only GET is offered — this tool
// holds read-only scopes and never mutates.
func newAPICmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "api",
		Short: "Raw read-only API access (GET any v4 path)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}
	cmd.AddCommand(newAPIGetCmd(app))
	return cmd
}

func newAPIGetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "get <path>",
		Short: "GET a v4 API path, e.g. /v4/users/me/profile",
		Args:  cobra.ExactArgs(1),
		Example: "  google-health-cli api get /v4/users/me/profile\n" +
			"  google-health-cli api get /v4/users/me/settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := app.apiClient(cmd.Context())
			if err != nil {
				return err
			}
			body, err := client.RawGet(cmd.Context(), args[0])
			if err != nil {
				return withCode(ExitAuth, err)
			}
			out := cmd.OutOrStdout()
			if json.Valid(body) {
				return writeJSON(out, body) // re-indent for readability.
			}
			fprintln(out, string(body)) // non-JSON: emit verbatim.
			return nil
		},
	}
}
