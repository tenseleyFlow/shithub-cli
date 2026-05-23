// SPDX-License-Identifier: AGPL-3.0-or-later

// Package release wires the `shithub release` subtree onto the root
// command. Every subcommand is a stub that prints a friendly "releases
// not yet supported on this shithub host (see shithub-S48)" message and
// exits 2; the full implementation is gated on shithub server sprint
// S48 (Releases), which is currently parked. The stubs let `--help`
// discovery and tab-completion work today so users see the planned
// surface and aren't surprised when the real flags land.
//
// Once S48 ships, replace each stub with the corresponding real
// implementation (see .docs/sprints/C17-release.md for the contract).
package release

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewCmd builds the `release` parent and attaches the eight planned
// subcommands as stubs.
//
// H4 (H22/H23): migrated to the shared cmdutil.NewDeferredCmd factory
// so every stub uniformly accepts -R / --hostname / --json / --jq /
// --template.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <command>",
		Short: "Manage releases (deferred until shithub-S48 ships)",
		Long: `Manage GitHub-style releases on shithub.

This command tree is registered today so the planned flag surface is
discoverable, but the underlying shithub release model (S48) is parked.
Every invocation exits 2 with a friendly notice until S48 lands.`,
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	stub := func(name, short string) *cobra.Command {
		return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
			Use: name, Short: short,
			Name: "shithub release " + name, Track: "shithub-S48",
		})
	}
	cmd.AddCommand(stub("create", "Create a new release (deferred)"))
	cmd.AddCommand(stub("list", "List releases (deferred)"))
	cmd.AddCommand(stub("view", "Show details of a release (deferred)"))
	cmd.AddCommand(stub("upload", "Upload assets to a release (deferred)"))
	cmd.AddCommand(stub("download", "Download release assets (deferred)"))
	cmd.AddCommand(stub("delete", "Delete a release (deferred)"))
	cmd.AddCommand(stub("delete-asset", "Delete a single asset from a release (deferred)"))
	cmd.AddCommand(stub("edit", "Edit an existing release (deferred)"))
	return cmd
}
