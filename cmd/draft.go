package cmd

import (
	"fmt"
	"time"

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
		ctx := cmd.Context()
		client, manager := newAuthenticatedClient()
		user, err := manager.User(ctx)
		if err != nil {
			return err
		}

		recipient := args[0]
		data, err := readMessageContent(args[1])
		if err != nil {
			return err
		}

		payload, err := buildDraftPayload(user, recipient, data, draftOptions{
			pid:       draftCreatePID,
			hasPID:    cmd.Flags().Changed("pid"),
			topic:     draftCreateTopic,
			hasTopic:  cmd.Flags().Changed("topic"),
			important: draftCreateImportant,
			noReply:   draftCreateNoReply,
		})
		if err != nil {
			return err
		}

		draft, err := client.CreateMessage(ctx, payload)
		if err != nil {
			return fmt.Errorf("creating draft: %w", err)
		}

		if jsonOutput {
			return printJSON(draft)
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
		ctx := cmd.Context()
		client, _ := newAuthenticatedClient()
		msgID, err := resolveMessageID(ctx, client, args[0])
		if err != nil {
			return err
		}

		sent, err := client.SendMessage(ctx, msgID)
		if err != nil {
			return fmt.Errorf("sending draft: %w", err)
		}

		if jsonOutput {
			return printJSON(sent)
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
