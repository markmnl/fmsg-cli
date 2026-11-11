package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var getDataOriginal bool

var getDataCmd = &cobra.Command{
	Use:   "get-data <message-id> [output-file]",
	Short: "Download the body data of a message",
	Long: `Download the body data of a message, to output-file or stdout.

An edited message (FMSG-008) downloads as edited; --original downloads the
data as first sent, which stays stored.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		messageID := args[0]

		client, _ := newAuthenticatedClient()
		resolvedID, err := resolveMessageID(client, messageID)
		if err != nil {
			return err
		}
		messageIDStr := strconv.FormatInt(resolvedID, 10)
		if len(args) == 2 {
			outputPath := args[1]
			if err := client.DownloadData(messageIDStr, getDataOriginal, outputPath); err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(map[string]string{"id": messageIDStr, "saved_to": outputPath})
			}

			fmt.Printf("Data saved to %s\n", outputPath)
			return nil
		}

		if err := client.DownloadDataToWriter(messageIDStr, getDataOriginal, os.Stdout); err != nil {
			return err
		}
		return nil
	},
}

func init() {
	getDataCmd.Flags().BoolVar(&getDataOriginal, "original", false, "Download the data as first sent, ignoring any edit")
	rootCmd.AddCommand(getDataCmd)
}
