// Package cmd defines all CLI commands for the fmsg application.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fmsg",
	Short: "fmsg — command-line interface to an fmsg messaging server",
	Long: `fmsg is a CLI that communicates with an fmsg-webapi server.

Before using any command, authenticate with:

  fmsg login`,
}

// Execute runs the root command and exits on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
