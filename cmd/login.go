package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [address]",
	Short: "Authenticate and store a local token",
	Long: `Prompt for your FMSG address, generate a JWT token, and store it locally.

The token is stored in $XDG_CONFIG_HOME/fmsg/auth.json (or ~/.config/fmsg/auth.json)
and is valid for 24 hours.

Optionally supply the address as an argument to skip the prompt.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var user string

		if len(args) > 0 {
			user = args[0]
		} else {
			fmt.Print("FMSG address (e.g. @user@example.com): ")

			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading input: %w", err)
			}
			user = strings.TrimSpace(input)
		}

		if user == "" {
			return fmt.Errorf("FMSG address must not be empty")
		}

		token, exp, err := auth.Generate(user)
		if err != nil {
			return fmt.Errorf("generating token: %w", err)
		}

		creds := auth.Credentials{
			Token:     token,
			ExpiresAt: exp,
			User:      user,
		}
		if err := auth.Save(creds); err != nil {
			return fmt.Errorf("saving credentials: %w", err)
		}

		fmt.Printf("Logged in as %s (token expires %s)\n", user, exp.Format("2006-01-02T15:04:05Z"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
