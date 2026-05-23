// SPDX-License-Identifier: AGPL-3.0-or-later

// Package gpgkey wires the `shithub gpg-key` subtree onto the root.
package gpgkey

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	gpgkeyadd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/gpgkey/add"
	gpgkeydel "github.com/tenseleyFlow/shithub-cli/pkg/cmd/gpgkey/delete"
	gpgkeylist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/gpgkey/list"
)

// NewCmd builds the parent gpg-key command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gpg-key <command>",
		Short: "Manage your shithub GPG public keys",
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(gpgkeyadd.NewCmd(f))
	cmd.AddCommand(gpgkeylist.NewCmd(f))
	cmd.AddCommand(gpgkeydel.NewCmd(f))
	return cmd
}
