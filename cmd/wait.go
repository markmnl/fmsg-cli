package cmd

import (
	"fmt"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	waitSinceID int64
	waitTimeout int
)

var waitCmd = &cobra.Command{
	Use:   "wait",
	Short: "Long-poll for new messages",
	Long: `Long-poll until a new message arrives for the authenticated user.

Returns immediately when a new message is available, or exits after timeout when none arrive.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if waitSinceID < 0 {
			return fmt.Errorf("--since-id must be >= 0")
		}
		if waitTimeout < 1 || waitTimeout > 60 {
			return fmt.Errorf("--timeout must be between 1 and 60 seconds")
		}

		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		client := api.New(config.GetAPIURL(), creds.Token)
		result, err := client.WaitForMessage(waitSinceID, waitTimeout)
		if err != nil {
			return err
		}

		if result.HasNew {
			fmt.Printf("New message available. Latest ID: %d\n", result.LatestID)
			return nil
		}

		fmt.Println("No new messages.")
		return nil
	},
}

func init() {
	waitCmd.Flags().Int64Var(&waitSinceID, "since-id", 0, "Only consider messages with ID greater than this value")
	waitCmd.Flags().IntVar(&waitTimeout, "timeout", 25, "Maximum seconds to wait (1-60)")
	rootCmd.AddCommand(waitCmd)
}
