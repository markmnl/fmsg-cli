package cmd

import (
	"fmt"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach <message-id> <file>",
	Short: "Upload a file as an attachment to a message",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		messageID := args[0]
		filePath := args[1]

		client := api.New(config.GetAPIURL(), creds.Token)
		if err := client.UploadAttachment(messageID, filePath); err != nil {
			return err
		}

		fmt.Printf("Attachment uploaded to message %s\n", messageID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(attachCmd)
}
