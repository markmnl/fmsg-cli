package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

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
  
Only provided fields are updated; recipients in to are fully replaced.
(The API's PUT replaces the whole draft, so the current draft is fetched
and merged first — unchanged fields are preserved.)`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, manager := newAuthenticatedClient()
		user, err := manager.User(cmd.Context())
		if err != nil {
			return err
		}

		msgID, err := resolveMessageID(client, args[0])
		if err != nil {
			return err
		}

		// The API's PUT replaces the whole draft: fetch the current state and
		// merge, so fields the caller didn't provide are preserved rather
		// than wiped.
		idStr := strconv.FormatInt(msgID, 10)
		existing, err := client.GetMessage(idStr)
		if err != nil {
			return fmt.Errorf("fetching current draft: %w", err)
		}

		msg := map[string]interface{}{
			"from":    user,
			"version": 1,
		}

		to := existing.To
		if len(updateTo) > 0 {
			to = updateTo
		}
		if len(to) > 0 {
			msg["to"] = to
		}

		topic := existing.Topic
		if cmd.Flags().Changed("topic") {
			topic = updateTopic
		}
		if topic != "" {
			msg["topic"] = topic
		}

		if cmd.Flags().Changed("pid") {
			msg["pid"] = updatePID
		} else if existing.PID != nil {
			msg["pid"] = *existing.PID
		}

		important := existing.Important
		if cmd.Flags().Changed("important") {
			important = updateImportant
		}
		msg["important"] = important

		noReply := existing.NoReply
		if cmd.Flags().Changed("no-reply") {
			noReply = updateNoReply
		}
		msg["no_reply"] = noReply

		var data []byte
		if len(args) == 2 {
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
		} else if existing.Size > 0 {
			var buf bytes.Buffer
			if err := client.DownloadDataToWriter(idStr, &buf); err != nil {
				return fmt.Errorf("fetching current draft body: %w", err)
			}
			data = buf.Bytes()
		}
		msg["data"] = string(data)
		msg["size"] = len(data)

		typ := existing.Type
		if cmd.Flags().Changed("type") {
			typ = updateType
		}
		if typ == "" && len(args) == 2 {
			typ = "text/plain"
		}
		if typ != "" {
			msg["type"] = typ
		}

		payload, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("encoding message: %w", err)
		}

		if err := client.UpdateMessage(msgID, payload); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(map[string]int64{"id": msgID})
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
