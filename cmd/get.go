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
		fmt.Printf("ID:   %s\n", msg.ID)
		fmt.Printf("From: %s\n", msg.From)
		fmt.Printf("To:   %s\n", string(to))
		if len(msg.Data) > 0 && string(msg.Data) != "null" {
			var pretty interface{}
			if err := json.Unmarshal(msg.Data, &pretty); err == nil {
				b, _ := json.MarshalIndent(pretty, "", "  ")
				fmt.Printf("Data:\n%s\n", string(b))
			} else {
				fmt.Printf("Data: %s\n", string(msg.Data))
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
