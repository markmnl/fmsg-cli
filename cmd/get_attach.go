package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var getAttachCmd = &cobra.Command{
	Use:   "get-attach <message-id> <filename> <output-file>",
	Short: "Download an attachment from a message",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		resolvedID, err := resolveMessageID(client, args[0])
		if err != nil {
			return err
		}
		messageID := strconv.FormatInt(resolvedID, 10)
		filename := args[1]
		outputPath := args[2]
		if err := client.DownloadAttachment(messageID, filename, outputPath); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(map[string]string{"filename": filename, "saved_to": outputPath})
		}

		fmt.Printf("Attachment saved to %s\n", outputPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getAttachCmd)
}
