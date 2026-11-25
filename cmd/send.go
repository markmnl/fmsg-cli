package cmd

import (
	"fmt"
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
		ctx := cmd.Context()
		client, manager := newAuthenticatedClient()
		user, err := manager.User(ctx)
		if err != nil {
			return err
		}

		recipient := args[0]
		data, err := readMessageContent(cmd.Context(), args[1])
		if err != nil {
			return err
		}

		payload, err := buildDraftPayload(user, recipient, data, draftOptions{
			pid:       sendPID,
			hasPID:    cmd.Flags().Changed("pid"),
			topic:     sendTopic,
			hasTopic:  cmd.Flags().Changed("topic"),
			important: sendImportant,
			noReply:   sendNoReply,
		})
		if err != nil {
			return err
		}

		draft, err := client.CreateMessage(ctx, payload)
		if err != nil {
			return fmt.Errorf("creating draft: %w", err)
		}

		sent, err := client.SendMessage(ctx, draft.ID)
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
