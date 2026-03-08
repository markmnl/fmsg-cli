package cmd

import (
	"fmt"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var rmAttachCmd = &cobra.Command{
	Use:   "rm-attach <message-id> <filename>",
	Short: "Remove an attachment from a message",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		messageID := args[0]
		filename := args[1]

		client := api.New(config.GetAPIURL(), creds.Token)
		if err := client.DeleteAttachment(messageID, filename); err != nil {
			return err
		}

		fmt.Printf("Attachment %s removed from message %s\n", filename, messageID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rmAttachCmd)
}
