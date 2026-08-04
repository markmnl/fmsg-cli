package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var rmAttachCmd = &cobra.Command{
	Use:   "rm-attach <message-id> <filename>",
	Short: "Remove an attachment from a message",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		resolvedID, err := resolveMessageID(client, args[0])
		if err != nil {
			return err
		}
		messageID := strconv.FormatInt(resolvedID, 10)
		filename := args[1]
		if err := client.DeleteAttachment(messageID, filename); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(map[string]string{"id": messageID, "filename": filename})
		}

		fmt.Printf("Attachment %s removed from message %s\n", filename, messageID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rmAttachCmd)
}
