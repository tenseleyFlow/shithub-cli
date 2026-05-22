// SPDX-License-Identifier: AGPL-3.0-or-later

// Package gist wires the `shithub gist` subtree onto the root command.
// Every subcommand is a stub that prints a friendly "gists not yet
// supported on this shithub host (see shithub-S49)" message and exits 2;
// the full implementation is gated on shithub server sprint S49 (Gists),
// which is currently parked. The stubs let `--help` discovery and
// tab-completion work today so users see the planned surface, and the
// CLI doesn't lock any wire-shape choices ahead of shithub's design.
//
// Once S49 ships, drop the typed client + real RunE bodies into place
// — the verb tree stays exactly as it is, see .docs/sprints/C20-gist.md
// for the contract.
package gist

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewCmd builds the `gist` parent and attaches the seven planned
// subcommands as stubs. H4 migration: shared cmdutil.NewDeferredCmd
// factory now handles -R / --hostname / --json / --jq / --template.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gist <command>",
		Short: "Manage gists (deferred until shithub-S49 ships)",
		Long: `Manage GitHub-style gists on shithub.

This command tree is registered today so the planned flag surface is
discoverable, but the underlying shithub gist model (S49) is parked.
Every invocation exits 2 with a friendly notice until S49 lands.`,
	}
	stub := func(name, short string) *cobra.Command {
		return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
			Use: name, Short: short,
			Name: "shithub gist " + name, Track: "shithub-S49",
		})
	}
	cmd.AddCommand(stub("create", "Create a new gist from files or stdin (deferred)"))
	cmd.AddCommand(stub("list", "List your gists (deferred)"))
	cmd.AddCommand(stub("view", "Show a gist's metadata and contents (deferred)"))
	cmd.AddCommand(stub("edit", "Add, remove, or rewrite files in a gist (deferred)"))
	cmd.AddCommand(stub("clone", "Clone a gist as a local git repo (deferred)"))
	cmd.AddCommand(stub("delete", "Delete a gist (deferred)"))
	cmd.AddCommand(stub("rename", "Rename a file within a gist (deferred)"))
	return cmd
}
