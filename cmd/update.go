package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	updateTo        []string
	updateTopic     string
	updateType      string
	updateImportant bool
	updateNoReply   bool
	updatePID       int64
)

var updateCmd = &cobra.Command{
	Use:   "update <message-id> [file|text]",
	Short: "Update a draft message",
	Long: `Update a draft message by ID. Optionally provide message body as:
  - A path to a file (must exist on disk)
  - A text string
  - "-" to read from stdin
  
Only provided fields are updated; recipients in to are fully replaced.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		msgID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid message ID: %w", err)
		}

		msg := map[string]interface{}{
			"from":    creds.User,
			"version": 1,
		}

		if len(updateTo) > 0 {
			msg["to"] = updateTo
		}
		if cmd.Flags().Changed("topic") {
			msg["topic"] = updateTopic
		}
		if cmd.Flags().Changed("type") {
			msg["type"] = updateType
		}
		if cmd.Flags().Changed("pid") {
			msg["pid"] = updatePID
		}
		if cmd.Flags().Changed("important") {
			msg["important"] = updateImportant
		}
		if cmd.Flags().Changed("no-reply") {
			msg["no_reply"] = updateNoReply
		}

		if len(args) == 2 {
			var data []byte
			content := args[1]
			switch content {
			case "-":
				data, err = io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}
			default:
				if fileData, ferr := os.ReadFile(content); ferr == nil {
					data = fileData
				} else {
					data = []byte(content)
				}
			}
			msg["data"] = string(data)
			msg["size"] = len(data)
			if !cmd.Flags().Changed("type") {
				msg["type"] = "text/plain"
			}
		}

		payload, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("encoding message: %w", err)
		}

		client := api.New(config.GetAPIURL(), creds.Token)
		if err := client.UpdateMessage(msgID, payload); err != nil {
			return err
		}

		fmt.Printf("Message %d updated\n", msgID)
		return nil
	},
}

func init() {
	updateCmd.Flags().StringSliceVar(&updateTo, "to", nil, "primary recipients (replaces existing)")
	updateCmd.Flags().StringVar(&updateTopic, "topic", "", "thread topic")
	updateCmd.Flags().StringVar(&updateType, "type", "", "MIME type of the message body")
	updateCmd.Flags().Int64VarP(&updatePID, "pid", "p", 0, "parent message ID")
	updateCmd.Flags().BoolVar(&updateImportant, "important", false, "mark message as important")
	updateCmd.Flags().BoolVar(&updateNoReply, "no-reply", false, "indicate replies will be discarded")
	rootCmd.AddCommand(updateCmd)
}
