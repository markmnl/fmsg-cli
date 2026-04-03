package cmd

import (
	"fmt"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var getDataCmd = &cobra.Command{
	Use:   "get-data <message-id> <output-file>",
	Short: "Download the body data of a message",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		messageID := args[0]
		outputPath := args[1]

		client := api.New(config.GetAPIURL(), creds.Token)
		if err := client.DownloadData(messageID, outputPath); err != nil {
			return err
		}

		fmt.Printf("Data saved to %s\n", outputPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getDataCmd)
}
