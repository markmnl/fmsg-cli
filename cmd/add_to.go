package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var addToCmd = &cobra.Command{
	Use:   "add-to <message-id> <recipient> [recipient...]",
	Short: "Add additional recipients to an existing message",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		msgID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid message ID: %w", err)
		}

		recipients := args[1:]

		client := api.New(config.GetAPIURL(), creds.Token)
		result, err := client.AddRecipients(msgID, recipients)
		if err != nil {
			return err
		}

		fmt.Printf("Added %d recipient(s) to message %d\n", result.Added, result.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addToCmd)
}
