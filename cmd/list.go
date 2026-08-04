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
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List messages for the authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		messages, err := client.ListMessages(listLimit, listOffset)
		if err != nil {
			return err
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
			fmt.Printf("ID: %d  From: %s  To: %s\n", msg.ID, msg.From, string(to))
		}
		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "Max number of messages to return (1-100)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of messages to skip")
	rootCmd.AddCommand(listCmd)
}
