// Package cmd defines all CLI commands for the fmsg application.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"unicode"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fmsg",
	Short: "fmsg — command-line interface to an fmsg messaging server",
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
func printJSON(v interface{}) error {
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
	for _, arg := range os.Args[1:] {
		if arg == "--" {
			return // user already supplied the separator
		}
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

// Execute runs the root command and exits on error.
func Execute() {
	injectDashDash()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errNoEvent) {
			os.Exit(exitNoEvent)
		}
		os.Exit(1)
	}
}
