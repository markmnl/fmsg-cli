package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <message-id>",
	Short: "Retrieve a message by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		client := api.New(config.GetAPIURL(), creds.Token)
		msgID, err := resolveMessageID(client, args[0])
		if err != nil {
			return err
		}
		msg, err := client.GetMessage(strconv.FormatInt(msgID, 10))
		if err != nil {
			return err
		}

		to, _ := json.Marshal(msg.To)
		fmt.Printf("From: %s\n", msg.From)
		fmt.Printf("To:   %s\n", string(to))
		if len(msg.AddTo) > 0 {
			var flat []string
			for _, b := range msg.AddTo {
				flat = append(flat, b.To...)
			}
			addTo, _ := json.Marshal(flat)
			fmt.Printf("Add-To: %s\n", string(addTo))
			for _, b := range msg.AddTo {
				to, _ := json.Marshal(b.To)
				fmt.Printf("  added by %s at %f: %s\n", b.AddToFrom, b.Time, string(to))
			}
		}
		if msg.PID != nil {
			fmt.Printf("PID:  %d\n", *msg.PID)
		}
		if msg.Topic != "" {
			fmt.Printf("Topic: %s\n", msg.Topic)
		}
		fmt.Printf("Type: %s\n", msg.Type)
		fmt.Printf("Size: %d\n", msg.Size)
		if msg.Time != nil {
			fmt.Printf("Time: %f\n", *msg.Time)
		}
		if msg.Important {
			fmt.Println("Important: yes")
		}
		if msg.NoReply {
			fmt.Println("No-Reply: yes")
		}
		if len(msg.Attachments) > 0 {
			fmt.Println("Attachments:")
			for _, a := range msg.Attachments {
				fmt.Printf("  %s (%d bytes)\n", a.Filename, a.Size)
			}
		}
		if msg.ShortText != nil && *msg.ShortText != "" {
			fmt.Println()
			fmt.Println(*msg.ShortText)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
}
