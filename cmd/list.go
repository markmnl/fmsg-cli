package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
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
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		client := api.New(config.GetAPIURL(), creds.Token)
		messages, err := client.ListMessages(listLimit, listOffset)
		if err != nil {
			return err
		}

		if len(messages) == 0 {
			fmt.Println("No messages.")
			return nil
		}

		for _, msg := range messages {
			to, _ := json.Marshal(msg.To)
			fmt.Printf("ID: %s  From: %s  To: %s\n", msg.ID, msg.From, string(to))
		}
		return nil
	},
}

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", 20, "Max number of messages to return (1-100)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Number of messages to skip")
	rootCmd.AddCommand(listCmd)
}
