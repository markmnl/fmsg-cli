package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	draftCreatePID       int64
	draftCreateTopic     string
	draftCreateImportant bool
	draftCreateNoReply   bool
)

var draftCmd = &cobra.Command{
	Use:   "draft",
	Short: "Manage draft messages",
}

var draftCreateCmd = &cobra.Command{
	Use:   "create <recipient> <file|text>",
	Short: "Create a draft message without sending",
	Long: `Create a draft message for a recipient. The second argument can be:
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

		msg := map[string]interface{}{
			"from":    creds.User,
			"to":      []string{recipient},
			"version": 1,
			"type":    "text/plain",
			"size":    len(data),
			"data":    string(data),
		}
		if cmd.Flags().Changed("pid") {
			msg["pid"] = draftCreatePID
		}
		if cmd.Flags().Changed("topic") {
			msg["topic"] = draftCreateTopic
		}
		if draftCreateImportant {
			msg["important"] = true
		}
		if draftCreateNoReply {
			msg["no_reply"] = true
		}

		payload, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("encoding message: %w", err)
		}

		client := api.New(config.GetAPIURL(), creds.Token)
		draft, err := client.CreateMessage(payload)
		if err != nil {
			return fmt.Errorf("creating draft: %w", err)
		}

		fmt.Println("Draft created")
		fmt.Printf("ID: %d\n", draft.ID)
		return nil
	},
}

var draftSendCmd = &cobra.Command{
	Use:   "send <message-id>",
	Short: "Send a previously created draft",
	Args:  cobra.ExactArgs(1),
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

		client := api.New(config.GetAPIURL(), creds.Token)
		sent, err := client.SendMessage(msgID)
		if err != nil {
			return fmt.Errorf("sending draft: %w", err)
		}

		fmt.Println("Draft sent successfully")
		fmt.Printf("ID: %d\n", sent.ID)
		fmt.Printf("Time: %s\n", time.Unix(int64(sent.Time), 0).UTC().Format(time.RFC3339))
		return nil
	},
}

func init() {
	draftCreateCmd.Flags().Int64VarP(&draftCreatePID, "pid", "p", 0, "parent message ID (optional)")
	draftCreateCmd.Flags().StringVar(&draftCreateTopic, "topic", "", "thread topic (optional)")
	draftCreateCmd.Flags().BoolVar(&draftCreateImportant, "important", false, "mark message as important")
	draftCreateCmd.Flags().BoolVar(&draftCreateNoReply, "no-reply", false, "indicate replies will be discarded")

	draftCmd.AddCommand(draftCreateCmd)
	draftCmd.AddCommand(draftSendCmd)
	rootCmd.AddCommand(draftCmd)
}