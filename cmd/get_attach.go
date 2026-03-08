package cmd

import (
	"fmt"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var getAttachCmd = &cobra.Command{
	Use:   "get-attach <message-id> <filename> <output-file>",
	Short: "Download an attachment from a message",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		messageID := args[0]
		filename := args[1]
		outputPath := args[2]

		client := api.New(config.GetAPIURL(), creds.Token)
		if err := client.DownloadAttachment(messageID, filename, outputPath); err != nil {
			return err
		}

		fmt.Printf("Attachment saved to %s\n", outputPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getAttachCmd)
}
