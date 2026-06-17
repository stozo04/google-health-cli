// Command google-health-cli pulls elliptical cardio sessions from the Google
// Health API and upserts them into a personal-workout-ai DAILY_LOG.json. It is
// a self-contained, drop-in replacement for the Python google-health-cli: same
// commands, same --json contracts, same DAILY_LOG.json writes, but it owns its
// own OAuth2 + HTTP instead of shelling out to the `ghealth` binary. See GOAL.md
// for the full specification.
//
// This entrypoint is deliberately thin (GOAL.md §5): it owns the ldflags version
// vars (the linker's -X only reaches package main), wires a cancelable context
// for Ctrl-C, runs the cobra tree, and maps the resulting error to a single
// process exit code.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/stozo04/google-health-cli/internal/cli"
	"github.com/stozo04/google-health-cli/internal/version"
)

// Set by GoReleaser via -ldflags "-X main.versionString=... -X main.commit=...
// -X main.date=...". See .goreleaser.yaml and GOAL.md §18. Forwarded into the
// version package below.
var (
	versionString = ""
	commit        = ""
	date          = ""
)

func main() {
	os.Exit(run())
}

// run executes the program and returns a process exit code. Keeping the only
// os.Exit in main() guarantees deferred cleanup in commands still runs
// (GOAL.md §12).
func run() int {
	version.Set(versionString, commit, date)

	// Cancel the context on the first interrupt so in-flight HTTP requests can
	// abort cleanly; a second interrupt restores default behavior (hard kill).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	root := cli.NewRootCmd()
	err := root.ExecuteContext(ctx)
	if err == nil {
		return cli.ExitOK
	}

	// One error line to stderr; stdout stays clean (GOAL.md §12, §13). A command
	// that already produced its own output returns a message-less error, so skip
	// the line when there's nothing to say.
	if msg := err.Error(); msg != "" {
		fmt.Fprintln(os.Stderr, "google-health-cli: "+msg)
	}

	var exit *cli.ExitError
	if errors.As(err, &exit) {
		return exit.Code
	}
	return cli.ExitFailure
}
