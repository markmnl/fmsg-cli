package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
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
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
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
		payload, err := json.Marshal(map[string]interface{}{
			"from":    creds.User,
			"to":      []string{recipient},
			"version": 1,
			"flags":   0,
			"type":    "text/plain",
			"size":    len(data),
			"topic":   "",
			"data":    string(data),
		})
		if err != nil {
			return fmt.Errorf("encoding message: %w", err)
		}

		client := api.New(config.GetAPIURL(), creds.Token)

		draft, err := client.CreateMessage(payload)
		if err != nil {
			return fmt.Errorf("creating draft: %w", err)
		}

		if err := client.SendMessage(draft.ID); err != nil {
			return fmt.Errorf("sending message: %w", err)
		}

		fmt.Println("Message sent successfully")
		fmt.Printf("ID: %d\n", draft.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sendCmd)
}
