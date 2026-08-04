package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/spf13/cobra"
)

var (
	sentLimit  int
	sentOffset int
)

var sentCmd = &cobra.Command{
	Use:   "sent",
	Short: "List messages authored by the authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		messages, err := client.ListSentMessages(sentLimit, sentOffset)
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
			fmt.Println("No sent messages.")
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
	sentCmd.Flags().IntVar(&sentLimit, "limit", 20, "Max number of messages to return (1-100)")
	sentCmd.Flags().IntVar(&sentOffset, "offset", 0, "Number of messages to skip")
	rootCmd.AddCommand(sentCmd)
}
