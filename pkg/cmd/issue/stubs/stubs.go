// SPDX-License-Identifier: AGPL-3.0-or-later

// Package stubs registers the issue subcommands whose server contracts
// shithub hasn't shipped yet: pin / unpin / transfer / develop. Each
// returns a friendly "not yet supported" error so the subcommand is
// discoverable from `--help` rather than missing — that way users get a
// pointer to the relevant tracking sprint rather than a "command not
// found" silent failure.
//
// When shithub's server gains the corresponding endpoint, the stub for
// that subcommand promotes into its own package alongside the others.
package stubs

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// notYetSupportedRunE returns a Cobra RunE that always errors with the
// stable "not yet supported" message + the sprint identifier where the
// feature is tracked. Centralized so the wording is consistent.
func notYetSupportedRunE(name, sprint string) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		return notYetSupportedError{Name: name, Sprint: sprint}
	}
}

// notYetSupportedError carries the user-facing message. Exported as a
// type so tests can errors.As against it without grepping strings.
type notYetSupportedError struct{ Name, Sprint string }

func (e notYetSupportedError) Error() string {
	return "issue " + e.Name + ": not yet supported by shithub server (tracked in " + e.Sprint + ")"
}

// NewPinCmd builds the `shithub issue pin` stub. shithub's issue table
// needs a `pinned` column plus PUT/DELETE endpoints (tracked in S50 §3).
func NewPinCmd(_ *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "pin <number-or-url>",
		Short: "Pin an issue (deferred until shithub server adds pin support)",
		Args:  cobra.ExactArgs(1),
		RunE:  notYetSupportedRunE("pin", "S50 §3 pin/unpin"),
	}
}

// NewUnpinCmd builds the `shithub issue unpin` stub. Pairs with NewPinCmd.
func NewUnpinCmd(_ *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "unpin <number-or-url>",
		Short: "Unpin an issue (deferred until shithub server adds pin support)",
		Args:  cobra.ExactArgs(1),
		RunE:  notYetSupportedRunE("unpin", "S50 §3 pin/unpin"),
	}
}

// NewTransferCmd builds the `shithub issue transfer` stub. shithub
// doesn't have cross-repo issue transfer; the endpoint is sketched in
// S50 §3 but unimplemented.
func NewTransferCmd(_ *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "transfer <number> <target-repo>",
		Short: "Transfer an issue to another repo (deferred until shithub supports cross-repo transfer)",
		Args:  cobra.ExactArgs(2),
		RunE:  notYetSupportedRunE("transfer", "S50 §3 transfer"),
	}
}

// NewDevelopCmd builds the `shithub issue develop` stub. Requires
// issue↔branch linking which shithub doesn't yet model (lands alongside
// a server-side branch-tracking sprint).
func NewDevelopCmd(_ *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "develop <number>",
		Short: "Create or list branches linked to an issue (deferred)",
		Args:  cobra.MinimumNArgs(1),
		RunE:  notYetSupportedRunE("develop", "server-side branch linking"),
	}
}
