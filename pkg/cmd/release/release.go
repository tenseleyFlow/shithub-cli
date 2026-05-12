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
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// deferredMessage is what every release subcommand prints to stderr
// before exiting 2. Keep this single source of truth so the wording
// stays consistent — Cobra's --help text already documents the planned
// behavior, so users hitting this message know it's a "not yet"
// rather than a permanent absence.
const deferredMessage = "releases not yet supported on this shithub host (see shithub-S48); subcommand registered for future use"

// deferredExitCode is exit(2) per the C17 spec — distinct from exit(1)
// (genuine failures) so scripts can branch on "feature pending" without
// matching the message string.
const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder. Production uses os.Exit; tests assign a func that
// stores the code and returns, so the assertion can read the code
// without terminating the test process.
var exitFn = os.Exit

// runDeferred prints the friendly message and exits with the deferred
// code. Subcommands plumb this through cobra's RunE; we never return
// the error to cobra (exitFn short-circuits the process in production)
// but we still match RunE's signature so help / flag-parsing flow
// runs first.
func runDeferred(cmd *cobra.Command, _ []string) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
	exitFn(deferredExitCode)
	return nil
}

// NewCmd builds the `release` parent and attaches the eight planned
// subcommands as stubs.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release <command>",
		Short: "Manage releases (deferred until shithub-S48 ships)",
		Long: `Manage GitHub-style releases on shithub.

This command tree is registered today so the planned flag surface is
discoverable, but the underlying shithub release model (S48) is parked.
Every invocation exits 2 with a friendly notice until S48 lands.`,
	}
	cmd.AddCommand(newStub("create", "Create a new release (deferred)"))
	cmd.AddCommand(newStub("list", "List releases (deferred)"))
	cmd.AddCommand(newStub("view", "Show details of a release (deferred)"))
	cmd.AddCommand(newStub("upload", "Upload assets to a release (deferred)"))
	cmd.AddCommand(newStub("download", "Download release assets (deferred)"))
	cmd.AddCommand(newStub("delete", "Delete a release (deferred)"))
	cmd.AddCommand(newStub("delete-asset", "Delete a single asset from a release (deferred)"))
	cmd.AddCommand(newStub("edit", "Edit an existing release (deferred)"))
	return cmd
}

// newStub builds one deferred subcommand. We deliberately don't define
// any flags here — flag-shape compatibility with gh isn't useful until
// the underlying implementation lands, and emitting bogus help text
// for not-yet-supported flags would be more confusing than the bare
// "deferred" notice.
func newStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE:  runDeferred,
	}
}
