package cmd

import (
	"fmt"
	"os"

	"github.com/markmnl/fmsg-cli/internal/api"
	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var delCmd = &cobra.Command{
	Use:   "del <message-id>",
	Short: "Delete a message by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		creds, err := auth.LoadValid()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		client := api.New(config.GetAPIURL(), creds.Token)
		if err := client.DeleteMessage(args[0]); err != nil {
			return err
		}

		fmt.Printf("Message %s deleted\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(delCmd)
}
