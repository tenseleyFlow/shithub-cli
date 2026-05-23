// SPDX-License-Identifier: AGPL-3.0-or-later

// Package sshkey wires the `shithub ssh-key` subtree onto the root
// command. gh's spelling is `gh ssh-key` (with the hyphen); we mirror
// the cobra-level name while keeping the Go package import path
// hyphen-free (Go convention).
package sshkey

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	sshkeyadd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/sshkey/add"
	sshkeydel "github.com/tenseleyFlow/shithub-cli/pkg/cmd/sshkey/delete"
	sshkeylist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/sshkey/list"
)

// NewCmd builds the parent ssh-key command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-key <command>",
		Short: "Manage your shithub SSH public keys",
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(sshkeyadd.NewCmd(f))
	cmd.AddCommand(sshkeylist.NewCmd(f))
	cmd.AddCommand(sshkeydel.NewCmd(f))
	return cmd
}
