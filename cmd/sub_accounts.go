package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/markmnl/fmsg-cli/internal/api"
)

var (
	subAccountCreateCIDRs  string
	subAccountCreateExpiry string
	subAccountUpdateCIDRs  string
	subAccountRotateExpiry string
)

var subAccountsCmd = &cobra.Command{
	Use:   "sub-accounts",
	Short: "Manage sub-account API-access grants",
}

var subAccountsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sub-accounts owned by the authenticated user",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		list, err := client.ListSubAccounts()
		if err != nil {
			return err
		}

		if jsonOutput {
			if list.SubAccounts == nil {
				list.SubAccounts = []api.SubAccount{}
			}
			return printJSON(list)
		}

		if len(list.SubAccounts) == 0 {
			fmt.Printf("No sub-accounts (max %d).\n", list.MaxSubAccounts)
			return nil
		}

		fmt.Printf("Max sub-accounts: %d\n", list.MaxSubAccounts)
		for _, a := range list.SubAccounts {
			printSubAccount(a)
		}
		return nil
	},
}

var subAccountsGetCmd = &cobra.Command{
	Use:   "get <agent>",
	Short: "Retrieve a single sub-account grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		account, err := client.GetSubAccount(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(account)
		}
		printSubAccount(*account)
		return nil
	},
}

var subAccountsCreateCmd = &cobra.Command{
	Use:   "create <agent>",
	Short: "Derive a new sub-account and print its plaintext API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if subAccountCreateExpiry == "" {
			return fmt.Errorf("--expires is required")
		}
		cidrs := splitCIDRs(subAccountCreateCIDRs)
		if len(cidrs) == 0 {
			return fmt.Errorf("--cidr is required")
		}

		client, _ := newAuthenticatedClient()
		account, err := client.CreateSubAccount(args[0], cidrs, subAccountCreateExpiry)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(account)
		}

		printSubAccount(*account)
		fmt.Printf("API key (save now, shown once): %s\n", account.APIKey)
		return nil
	},
}

var subAccountsUpdateCIDRsCmd = &cobra.Command{
	Use:   "update-cidrs <agent>",
	Short: "Replace a sub-account's allowed CIDRs without rotating its key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cidrs := splitCIDRs(subAccountUpdateCIDRs)
		if len(cidrs) == 0 {
			return fmt.Errorf("--cidr is required")
		}

		client, _ := newAuthenticatedClient()
		account, err := client.UpdateSubAccountCIDRs(args[0], cidrs)
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(account)
		}
		printSubAccount(*account)
		return nil
	},
}

var subAccountsRotateKeyCmd = &cobra.Command{
	Use:   "rotate-key <agent>",
	Short: "Rotate a sub-account's API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if subAccountRotateExpiry == "" {
			return fmt.Errorf("--expires is required")
		}

		client, _ := newAuthenticatedClient()
		account, err := client.RotateSubAccountKey(args[0], subAccountRotateExpiry)
		if err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(account)
		}

		printSubAccount(*account)
		fmt.Printf("API key (save now, shown once): %s\n", account.APIKey)
		return nil
	},
}

var subAccountsDeleteCmd = &cobra.Command{
	Use:   "delete <agent>",
	Short: "Delete a sub-account grant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _ := newAuthenticatedClient()
		if err := client.DeleteSubAccount(args[0]); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(map[string]string{"agent": args[0]})
		}
		fmt.Printf("Sub-account %q deleted\n", args[0])
		return nil
	},
}

func printSubAccount(a api.SubAccount) {
	fmt.Printf("Agent: %s\n", a.Agent)
	fmt.Printf("  Addr: %s\n", a.Addr)
	fmt.Printf("  Grant type: %s\n", a.GrantType)
	if a.DisplayName != "" {
		fmt.Printf("  Display name: %s\n", a.DisplayName)
	}
	if a.KeyID != "" {
		fmt.Printf("  Key ID: %s\n", a.KeyID)
	}
	fmt.Printf("  Allowed CIDRs: %s\n", strings.Join(a.AllowedCIDRs, ", "))
	fmt.Printf("  Key expires at: %s\n", a.KeyExpiresAt)
}

func splitCIDRs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, c := range strings.Split(raw, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			out = append(out, c)
		}
	}
	return out
}

func init() {
	subAccountsCreateCmd.Flags().StringVar(&subAccountCreateCIDRs, "cidr", "", "comma-separated allowed CIDR ranges (required)")
	subAccountsCreateCmd.Flags().StringVar(&subAccountCreateExpiry, "expires", "", "API key expiry as RFC3339 timestamp (required)")

	subAccountsUpdateCIDRsCmd.Flags().StringVar(&subAccountUpdateCIDRs, "cidr", "", "comma-separated allowed CIDR ranges (required)")

	subAccountsRotateKeyCmd.Flags().StringVar(&subAccountRotateExpiry, "expires", "", "new API key expiry as RFC3339 timestamp (required)")

	subAccountsCmd.AddCommand(subAccountsListCmd)
	subAccountsCmd.AddCommand(subAccountsGetCmd)
	subAccountsCmd.AddCommand(subAccountsCreateCmd)
	subAccountsCmd.AddCommand(subAccountsUpdateCIDRsCmd)
	subAccountsCmd.AddCommand(subAccountsRotateKeyCmd)
	subAccountsCmd.AddCommand(subAccountsDeleteCmd)
	rootCmd.AddCommand(subAccountsCmd)
}
