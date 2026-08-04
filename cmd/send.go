package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	sendPID       int64
	sendTopic     string
	sendImportant bool
	sendNoReply   bool
)

var sendCmd = &cobra.Command{
	Use:   "send <recipient> <file|text>",
	Short: "Send a message to a recipient",
	Long: `Send a message to a recipient. The second argument can be:
  - A path to a file (must exist on disk)
  - A text string
 - "-" to read from stdin`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, manager := newAuthenticatedClient()
		user, err := manager.User(cmd.Context())
		if err != nil {
			return err
		}

		recipient := args[0]
		content := args[1]

		var data []byte

		switch content {
		case "-":
			// Read from stdin.
			data, err = io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
		default:
			// Try as file first; fall back to using content directly as text.
			if fileData, ferr := os.ReadFile(content); ferr == nil {
				data = fileData
			} else {
				data = []byte(content)
			}
		}

		// Build a draft payload.
		msg := map[string]interface{}{
			"from":    user,
			"to":      []string{recipient},
			"version": 1,
			"type":    "text/plain",
			"size":    len(data),
			"data":    string(data),
		}
		if cmd.Flags().Changed("pid") {
			msg["pid"] = sendPID
		}
		if cmd.Flags().Changed("topic") {
			msg["topic"] = sendTopic
		}
		if sendImportant {
			msg["important"] = true
		}
		if sendNoReply {
			msg["no_reply"] = true
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("encoding message: %w", err)
		}

		draft, err := client.CreateMessage(payload)
		if err != nil {
			return fmt.Errorf("creating draft: %w", err)
		}

		sent, err := client.SendMessage(draft.ID)
		if err != nil {
			return fmt.Errorf("sending message: %w", err)
		}

		if jsonOutput {
			return printJSON(sent)
		}

		fmt.Println("Message sent successfully")
		fmt.Printf("ID: %d\n", sent.ID)
		fmt.Printf("Time: %s\n", time.Unix(int64(sent.Time), 0).UTC().Format(time.RFC3339))
		return nil
	},
}

func init() {
	sendCmd.Flags().Int64VarP(&sendPID, "pid", "p", 0, "parent message ID (optional)")
	sendCmd.Flags().StringVar(&sendTopic, "topic", "", "thread topic (optional)")
	sendCmd.Flags().BoolVar(&sendImportant, "important", false, "mark message as important")
	sendCmd.Flags().BoolVar(&sendNoReply, "no-reply", false, "indicate replies will be discarded")
	rootCmd.AddCommand(sendCmd)
}
