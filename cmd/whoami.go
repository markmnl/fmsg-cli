package cmd

import (
	"fmt"
	"time"

	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Print the authenticated fmsg address and API URL",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiURL := config.GetAPIURL()
		manager := auth.NewManager(apiURL)
		tok, err := manager.Token(cmd.Context(), false)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(map[string]string{
				"address":          tok.User,
				"api_url":          apiURL,
				"token_expires_at": tok.ExpiresAt.UTC().Format(time.RFC3339),
			})
		}

		fmt.Printf("Address: %s\n", tok.User)
		fmt.Printf("API URL: %s\n", apiURL)
		fmt.Printf("Token expires: %s\n", tok.ExpiresAt.UTC().Format(time.RFC3339))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
