package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/spf13/cobra"
)

var (
	listLimit  int
	listOffset int
	listAll    bool
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List messages for the authenticated user",
	Long: `List messages for the authenticated user, newest first.

Edit messages (FMSG-008) are left out unless --all is given: the change each
one carries is already applied to the message it edits, which is marked
"(edited)". They are filtered after the server pages, so a page can hold
fewer than --limit rows.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		messages, err := client.ListMessages(listLimit, listOffset)
		if err != nil {
			return err
		}
		if !listAll {
			messages = api.VisibleMessages(messages)
		}

		if jsonOutput {
			if messages == nil {
				messages = []api.MessageListItem{}
			}
			return printJSON(messages)
		}

		if len(messages) == 0 {
			fmt.Println("No messages.")
			return nil
		}

		for _, msg := range messages {
			to, _ := json.Marshal(msg.To)
			fmt.Println(listLine(msg, string(to)))
		}
		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "Max number of messages to return (1-100)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of messages to skip")
	listCmd.Flags().BoolVar(&listAll, "all", false, "Include edit messages (FMSG-008), which are hidden by default")
	rootCmd.AddCommand(listCmd)
}

// listLine renders one list row; to is the already-encoded recipient list.
func listLine(msg api.MessageListItem, to string) string {
	line := fmt.Sprintf("ID: %d  From: %s  To: %s", msg.ID, msg.From, to)
	if msg.Edit {
		if msg.PID != nil {
			line += fmt.Sprintf("  (edit of %d)", *msg.PID)
		} else {
			line += "  (edit)"
		}
	}
	if msg.Edited != nil {
		line += "  (edited)"
	}
	return line
}
