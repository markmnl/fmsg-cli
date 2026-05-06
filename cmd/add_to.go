package cmd

import (
	"fmt"
	"os"

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

		client := api.New(config.GetAPIURL(), creds.Token)
		msgID, err := resolveMessageID(client, args[0])
		if err != nil {
			return err
		}

		recipients := args[1:]

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
