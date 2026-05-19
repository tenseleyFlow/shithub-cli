// SPDX-License-Identifier: AGPL-3.0-or-later

// Package stubs registers the issue subcommands whose server contracts
// shithub hasn't shipped yet: pin / unpin / transfer / develop. Each
// returns a friendly "not yet supported" error so the subcommand is
// discoverable from `--help` rather than missing — users get a pointer
// to the relevant tracking sprint rather than a "command not found"
// silent failure.
//
// Implementation rides cmdutil.NewDeferredCmd, which means stubs accept
// arbitrary flags (E-audit E20) and the standard --repo/-R flag is
// always present so users in a non-default cwd can still discover the
// deferred status (E21). When shithub's server gains the corresponding
// endpoint, the stub for that subcommand promotes into its own package
// alongside the others.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewPinCmd builds the `shithub issue pin` stub. shithub's issue table
// needs a `pinned` column plus PUT/DELETE endpoints (tracked in S50 §3).
func NewPinCmd(_ *cmdutil.Factory) *cobra.Command {
	return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
		Use:   "pin <number-or-url>",
		Short: "Pin an issue (deferred until shithub server adds pin support)",
		Name:  "issue pin",
		Track: "S50 §3 pin/unpin",
	})
}

// NewUnpinCmd builds the `shithub issue unpin` stub. Pairs with NewPinCmd.
func NewUnpinCmd(_ *cmdutil.Factory) *cobra.Command {
	return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
		Use:   "unpin <number-or-url>",
		Short: "Unpin an issue (deferred until shithub server adds pin support)",
		Name:  "issue unpin",
		Track: "S50 §3 pin/unpin",
	})
}

// NewTransferCmd builds the `shithub issue transfer` stub. shithub
// doesn't have cross-repo issue transfer; the endpoint is sketched in
// S50 §3 but unimplemented.
func NewTransferCmd(_ *cmdutil.Factory) *cobra.Command {
	return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
		Use:   "transfer <number> <target-repo>",
		Short: "Transfer an issue to another repo (deferred until shithub supports cross-repo transfer)",
		Name:  "issue transfer",
		Track: "S50 §3 transfer",
	})
}

// NewDevelopCmd builds the `shithub issue develop` stub. Requires
// issue↔branch linking which shithub doesn't yet model (lands alongside
// a server-side branch-tracking sprint).
func NewDevelopCmd(_ *cmdutil.Factory) *cobra.Command {
	return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
		Use:   "develop <number>",
		Short: "Create or list branches linked to an issue (deferred)",
		Name:  "issue develop",
		Track: "server-side branch linking",
	})
}
