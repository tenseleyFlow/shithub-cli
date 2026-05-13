// SPDX-License-Identifier: AGPL-3.0-or-later

// Package project wires the `shithub project` subtree onto the root
// command. Every subcommand is a stub that prints a friendly "projects
// not yet supported on this shithub host (see shithub-S47)" message and
// exits 2; the full implementation is gated on shithub server sprint
// S47 (Projects), which is currently parked.
//
// The verb tree mirrors gh's flat hyphenated naming (`item-list`,
// `field-list`, ...) so users typing what they know from gh find the
// commands here. Crucially, no types or client live in internal/ until
// shithub S47 lands and decides REST vs GraphQL — locking that choice
// ahead of the server design would be wasted work either direction. See
// .docs/sprints/C21-project.md for the contract.
package project

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

const deferredMessage = "projects not yet supported on this shithub host (see shithub-S47); subcommand registered for future use"

const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder. Same pattern as pkg/cmd/release and pkg/cmd/gist.
var exitFn = os.Exit

func runDeferred(cmd *cobra.Command, _ []string) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
	exitFn(deferredExitCode)
	return nil
}

// NewCmd builds the `project` parent and attaches the planned
// subcommands as stubs.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project <command>",
		Short: "Manage ProjectsV2-style projects (deferred until shithub-S47 ships)",
		Long: `Manage GitHub-ProjectsV2-style boards on shithub.

This command tree is registered today so the planned flag surface is
discoverable, but the underlying shithub project model (S47) is parked
and the transport choice (REST vs GraphQL) is undecided. Every
invocation exits 2 with a friendly notice until S47 lands.`,
	}
	// Direct verbs (9).
	cmd.AddCommand(newStub("list", "List projects for a user or org (deferred)"))
	cmd.AddCommand(newStub("view", "Show a project's details (deferred)"))
	cmd.AddCommand(newStub("create", "Create a new project (deferred)"))
	cmd.AddCommand(newStub("edit", "Edit a project's metadata (deferred)"))
	cmd.AddCommand(newStub("close", "Close a project (deferred)"))
	cmd.AddCommand(newStub("copy", "Copy a project (deferred)"))
	cmd.AddCommand(newStub("delete", "Delete a project (deferred)"))
	cmd.AddCommand(newStub("link", "Link a project to a repository (deferred)"))
	cmd.AddCommand(newStub("unlink", "Unlink a project from a repository (deferred)"))
	// Item verbs (6, flat-hyphenated to match gh).
	cmd.AddCommand(newStub("item-list", "List items in a project (deferred)"))
	cmd.AddCommand(newStub("item-add", "Add an existing issue or PR to a project (deferred)"))
	cmd.AddCommand(newStub("item-create", "Create a draft-note item in a project (deferred)"))
	cmd.AddCommand(newStub("item-edit", "Edit a project item's field values (deferred)"))
	cmd.AddCommand(newStub("item-archive", "Archive a project item (deferred)"))
	cmd.AddCommand(newStub("item-delete", "Delete a project item (deferred)"))
	// Field verbs (3).
	cmd.AddCommand(newStub("field-list", "List fields in a project (deferred)"))
	cmd.AddCommand(newStub("field-create", "Create a new field in a project (deferred)"))
	cmd.AddCommand(newStub("field-delete", "Delete a field from a project (deferred)"))
	// Template verbs (1). The C21 spec spells this `project template
	// list`; we ship the flat `template-list` form so all stubs share
	// the same depth, then revisit nesting when S47's template surface
	// firms up (it may grow create/delete verbs).
	cmd.AddCommand(newStub("template-list", "List project templates available to a user or org (deferred)"))
	return cmd
}

// newStub builds one deferred subcommand. No flags here — flag-shape
// compatibility isn't useful until the implementation lands, and the
// REST-vs-GraphQL transport question may yet reshape some flags.
func newStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE:  runDeferred,
	}
}
