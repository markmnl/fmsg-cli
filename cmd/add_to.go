package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addToCmd = &cobra.Command{
	Use:   "add-to <message-id> <recipient> [recipient...]",
	Short: "Add additional recipients to an existing message",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
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
