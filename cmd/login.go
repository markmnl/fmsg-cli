package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
			input, err := readCredential(cmd.Context())
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

// readCredential prompts for a credential on stdin. The read is abandoned
// with ctx's error when ctx is cancelled (Ctrl-C at the prompt).
func readCredential(ctx context.Context) (string, error) {
	fmt.Print("fmsg API key or user JWT: ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		// ReadPassword turns echo off and only restores it when it returns,
		// so put the terminal back ourselves if the prompt is abandoned.
		state, _ := term.GetState(fd)
		input, err := awaitContext(ctx, func() ([]byte, error) { return term.ReadPassword(fd) })
		if err != nil && ctx.Err() != nil && state != nil {
			_ = term.Restore(fd, state)
		}
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("reading API key: %w", err)
		}
		return string(input), nil
	}

	return awaitContext(ctx, func() (string, error) { return readCredentialFrom(os.Stdin) })
}

func readCredentialFrom(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && len(input) > 0) {
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
