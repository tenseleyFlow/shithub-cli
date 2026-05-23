// SPDX-License-Identifier: AGPL-3.0-or-later

// Package secret wires the `shithub secret` subtree onto root.
package secret

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	secretdelete "github.com/tenseleyFlow/shithub-cli/pkg/cmd/secret/delete"
	secretlist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/secret/list"
	secretset "github.com/tenseleyFlow/shithub-cli/pkg/cmd/secret/set"
)

// NewCmd builds the parent.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret <command>",
		Short: "Manage GitHub-Actions-style secrets",
		// I27 (discoverability half): secrets are write-only by design
		// (matches gh). `secret get` doesn't exist; users should use
		// `secret list` to enumerate names + `secret set` to rotate.
		Long: `Manage repository, organization, and environment secrets.

Secrets are write-only — there's no read endpoint exposing the cleartext
value. Use 'secret list' to enumerate names, 'secret set' to add or
rotate, and 'secret delete' to remove.`,
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(secretlist.NewCmd(f))
	cmd.AddCommand(secretset.NewCmd(f))
	cmd.AddCommand(secretdelete.NewCmd(f))
	return cmd
}
