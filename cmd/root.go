// Package cmd defines all CLI commands for the fmsg application.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"syscall"
	"unicode"

	"github.com/spf13/cobra"
)

// version is set by release builds with
// -ldflags "-X github.com/markmnl/fmsg-cli/cmd.version=v1.2.3".
var version = ""

// buildVersion reports the release version, falling back to the module
// version recorded by `go install ...@version`, then "dev".
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

var rootCmd = &cobra.Command{
	Use:     "fmsg",
	Version: buildVersion(),
	Short:   "fmsg — command-line interface to an fmsg messaging server",
	Long: `fmsg is a CLI that communicates with an fmsg-webapi server.

Before using any command, authenticate with:

  fmsg login [api-key|jwt]`,
}

// jsonOutput is set by the persistent --json flag. When true, commands emit a
// single JSON value on stdout instead of human-readable text. Commands whose
// primary output is raw data (get-data to stdout) are unaffected, and errors
// are still plain text on stderr.
var jsonOutput bool

// printJSON writes v to stdout as a single line of JSON.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// injectDashDash inserts a "--" sentinel into os.Args immediately before the
// first argument that looks like a negative integer (e.g. "-1", "-2").  This
// lets users write `fmsg get -1` without the shell/pflag flag-parser
// misinterpreting the leading dash as a shorthand flag.  If "--" is already
// present in os.Args the function is a no-op.
func injectDashDash() {
	if slices.Contains(os.Args[1:], "--") {
		return // user already supplied the separator
	}
	for i, arg := range os.Args[1:] {
		if len(arg) >= 2 && arg[0] == '-' && unicode.IsDigit(rune(arg[1])) {
			// Insert "--" at position i+1 in os.Args (i is 0-based in os.Args[1:]).
			pos := i + 1
			newArgs := make([]string, len(os.Args)+1)
			copy(newArgs, os.Args[:pos])
			newArgs[pos] = "--"
			copy(newArgs[pos+1:], os.Args[pos:])
			os.Args = newArgs
			return
		}
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "emit machine-readable JSON output")
}

// exitInterrupted is the exit code after SIGINT/SIGTERM cancelled a command:
// 128 + SIGINT, what a shell reports for a process the signal killed.
const exitInterrupted = 130

// silenceOnInterrupt wraps every RunE under cmd so a command that fails
// because a signal cancelled its context exits quietly, as a process killed
// by the signal would, instead of printing "context canceled" and its usage.
func silenceOnInterrupt(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		silenceOnInterrupt(sub)
	}
	run := cmd.RunE
	if run == nil {
		return
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		err := run(c, args)
		if err != nil && c.Context().Err() != nil && errors.Is(err, context.Canceled) {
			c.SilenceErrors, c.SilenceUsage = true, true
		}
		return err
	}
}

// Execute runs the root command and exits on error.
func Execute() {
	injectDashDash()
	silenceOnInterrupt(rootCmd)
	// SIGINT/SIGTERM cancel the command context so commands can clean up
	// (e.g. remove a partial download) instead of being killed mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal, restore default handling so a second one
	// terminates a command that is slow to notice the cancellation.
	context.AfterFunc(ctx, stop)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		interrupted := ctx.Err() != nil && errors.Is(err, context.Canceled)
		stop()
		if interrupted {
			os.Exit(exitInterrupted)
		}
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errNoEvent) {
			os.Exit(exitNoEvent)
		}
		os.Exit(1)
	}
}
