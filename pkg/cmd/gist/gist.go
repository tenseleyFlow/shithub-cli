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
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// deferredMessage is the single source of truth printed to stderr.
const deferredMessage = "gists not yet supported on this shithub host (see shithub-S49); subcommand registered for future use"

// deferredExitCode is exit(2) — distinct from exit(1) (genuine failures)
// so scripts can branch on "feature pending" without string matching.
const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder.  Same pattern as pkg/cmd/release.
var exitFn = os.Exit

func runDeferred(cmd *cobra.Command, _ []string) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
	exitFn(deferredExitCode)
	return nil
}

// NewCmd builds the `gist` parent and attaches the seven planned
// subcommands as stubs.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gist <command>",
		Short: "Manage gists (deferred until shithub-S49 ships)",
		Long: `Manage GitHub-style gists on shithub.

This command tree is registered today so the planned flag surface is
discoverable, but the underlying shithub gist model (S49) is parked.
Every invocation exits 2 with a friendly notice until S49 lands.`,
	}
	cmd.AddCommand(newStub("create", "Create a new gist from files or stdin (deferred)"))
	cmd.AddCommand(newStub("list", "List your gists (deferred)"))
	cmd.AddCommand(newStub("view", "Show a gist's metadata and contents (deferred)"))
	cmd.AddCommand(newStub("edit", "Add, remove, or rewrite files in a gist (deferred)"))
	cmd.AddCommand(newStub("clone", "Clone a gist as a local git repo (deferred)"))
	cmd.AddCommand(newStub("delete", "Delete a gist (deferred)"))
	cmd.AddCommand(newStub("rename", "Rename a file within a gist (deferred)"))
	return cmd
}

// newStub builds one deferred subcommand. We deliberately don't define
// flags — flag-shape compatibility isn't useful until the implementation
// lands, and stubbed flag help would mislead more than the bare notice.
func newStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE:  runDeferred,
	}
}
