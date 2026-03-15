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

var getCmd = &cobra.Command{
	Use:   "get <message-id>",
	Short: "Retrieve a message by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		client := api.New(config.GetAPIURL(), creds.Token)
		msg, err := client.GetMessage(args[0])
		if err != nil {
			return err
		}

		to, _ := json.Marshal(msg.To)
		fmt.Printf("From: %s\n", msg.From)
		fmt.Printf("To:   %s\n", string(to))
		if msg.Topic != "" {
			fmt.Printf("Topic: %s\n", msg.Topic)
		}
		fmt.Printf("Type: %s\n", msg.Type)
		fmt.Printf("Size: %d\n", msg.Size)
		if len(msg.Attachments) > 0 {
			fmt.Println("Attachments:")
			for _, a := range msg.Attachments {
				fmt.Printf("  %s (%d bytes)\n", a.Filename, a.Size)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
