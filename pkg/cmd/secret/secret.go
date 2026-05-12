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
	}
	cmd.AddCommand(secretlist.NewCmd(f))
	cmd.AddCommand(secretset.NewCmd(f))
	cmd.AddCommand(secretdelete.NewCmd(f))
	return cmd
}
