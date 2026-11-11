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
	sentAll    bool
)

var sentCmd = &cobra.Command{
	Use:   "sent",
	Short: "List messages authored by the authenticated user",
	Long: `List messages authored by the authenticated user, newest first.

Edit messages (FMSG-008) are left out unless --all is given: the change each
one carries is already applied to the message it edits, which is marked
"(edited)". They are filtered after the server pages, so a page can hold
fewer than --limit rows.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		messages, err := client.ListSentMessages(sentLimit, sentOffset)
		if err != nil {
			return err
		}
		if !sentAll {
			messages = api.VisibleMessages(messages)
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
			fmt.Println(listLine(msg, string(to)))
		}
		return nil
	},
}

func init() {
	sentCmd.Flags().IntVar(&sentLimit, "limit", 20, "Max number of messages to return (1-100)")
	sentCmd.Flags().IntVar(&sentOffset, "offset", 0, "Number of messages to skip")
	sentCmd.Flags().BoolVar(&sentAll, "all", false, "Include edit messages (FMSG-008), which are hidden by default")
	rootCmd.AddCommand(sentCmd)
}
