package cmd

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [address]",
	Short: "Authenticate and store a local token",
	Long: `Prompt for your fmsg address, generate a JWT token, and store it locally.

The token is stored in $XDG_CONFIG_HOME/fmsg/auth.json (or ~/.config/fmsg/auth.json)
and is valid for 24 hours.

Optionally supply the address as an argument to skip the prompt.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var user string
		exampleDomain := "example.com"
		if parsed, err := url.Parse(config.GetAPIURL()); err == nil {
			hostname := parsed.Hostname()
			if hostname != "" {
				exampleDomain = hostname
				parts := strings.Split(hostname, ".")
				if len(parts) > 2 && net.ParseIP(hostname) == nil {
					exampleDomain = strings.Join(parts[1:], ".")
				}
			}
		}

		if len(args) > 0 {
			user = strings.TrimSpace(args[0])
		} else {
			fmt.Printf("fmsg address or just user (e.g. @\x1b[1;36muser\x1b[0m@%s): ", exampleDomain)

			reader := bufio.NewReader(os.Stdin)
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading input: %w", err)
			}
			user = strings.TrimSpace(input)
		}

		if user == "" {
			return fmt.Errorf("fmsg address must not be empty")
		}
		if !strings.Contains(user, "@") {
			user = fmt.Sprintf("@%s@%s", user, exampleDomain)
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
		fmt.Printf("API URL: %s\n", config.GetAPIURL())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
