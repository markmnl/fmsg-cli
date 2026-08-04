package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var delCmd = &cobra.Command{
	Use:   "del <message-id>",
	Short: "Delete a message by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		msgID, err := resolveMessageID(client, args[0])
		if err != nil {
			return err
		}
		if err := client.DeleteMessage(msgID); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(map[string]int64{"id": msgID})
		}

		fmt.Printf("Message %d deleted\n", msgID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(delCmd)
}
