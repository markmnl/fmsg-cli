package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

var editRevert bool

var editCmd = &cobra.Command{
	Use:   "edit <message-id> [file|text|-]",
	Short: "Edit the body of a message you sent, or revert it to the original",
	Long: `Edit the body of a message you sent (FMSG-008).

The message content replaces the whole body and can be:
  - A path to a file (the file contents are used)
  - A text string (used directly)
  - "-" to read from stdin

Use --revert instead of content to show the message as first sent.
Participants see the new text marked as edited; the original stays stored
and visible to them. Drafts are changed with "fmsg update" instead.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if editRevert == (len(args) == 2) {
			return errors.New("give the new content, or --revert, but not both")
		}

		var data *string
		if !editRevert {
			raw, err := readMessageContent(args[1])
			if err != nil {
				return err
			}
			text := string(raw)
			data = &text
		}

		ctx := cmd.Context()
		client, _ := newAuthenticatedClient()
		msgID, err := resolveMessageID(ctx, client, args[0])
		if err != nil {
			return err
		}
		result, err := client.EditMessage(ctx, msgID, data)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(result)
		}
		switch {
		case !result.Sent:
			fmt.Printf("Message %d already reads that way; nothing sent\n", msgID)
		case editRevert:
			fmt.Printf("Reverted message %d to its original body\n", msgID)
		default:
			fmt.Printf("Edited message %d\n", msgID)
		}
		return nil
	},
}

func init() {
	editCmd.Flags().BoolVar(&editRevert, "revert", false, "revert the message to its original body")
	rootCmd.AddCommand(editCmd)
}
