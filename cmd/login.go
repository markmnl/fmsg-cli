package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/markmnl/fmsg-cli/internal/auth"
	"github.com/markmnl/fmsg-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var loginCmd = &cobra.Command{
	Use:   "login [api-key|jwt]",
	Short: "Authenticate with an API key or user JWT",
	Long: `Prompt for an fmsg API key or user JWT and store credentials locally.

API keys are exchanged for short-lived access tokens and refreshed automatically
before expiry. User JWTs are used directly until their JWT expiry.

Credentials are stored in $XDG_CONFIG_HOME/fmsg/auth.json (or ~/.config/fmsg/auth.json)
with 0600 permissions.

Optionally supply the credential as an argument to skip the prompt.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var credential string
		if len(args) > 0 {
			credential = strings.TrimSpace(args[0])
		} else {
			input, err := readCredential()
			if err != nil {
				return err
			}
			credential = strings.TrimSpace(input)
		}

		manager := auth.NewManager(config.GetAPIURL())
		var (
			tok auth.Token
			err error
		)
		if looksLikeJWT(credential) {
			tok, err = manager.LoginJWT(credential, loginAddress)
		} else {
			tok, err = manager.Login(cmd.Context(), credential)
		}
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(map[string]string{
				"user":       tok.User,
				"expires_at": tok.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
				"api_url":    config.GetAPIURL(),
			})
		}

		fmt.Printf("Logged in as %s (token expires %s)\n", tok.User, tok.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"))
		fmt.Printf("API URL: %s\n", config.GetAPIURL())
		return nil
	},
}

var loginAddress string

func readCredential() (string, error) {
	fmt.Print("fmsg API key or user JWT: ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		input, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("reading API key: %w", err)
		}
		return string(input), nil
	}

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading API key: %w", err)
	}
	return input, nil
}

func looksLikeJWT(s string) bool {
	return strings.Count(s, ".") == 2
}

func init() {
	loginCmd.Flags().StringVar(&loginAddress, "address", "", "fmsg address for user JWTs when the token does not contain one")
	rootCmd.AddCommand(loginCmd)
}
