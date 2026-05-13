// SPDX-License-Identifier: AGPL-3.0-or-later

// Package attestation wires the `shithub attestation` subtree onto
// the root command. Sigstore-style artifact attestation is fully gated
// on a shithub server-side Sigstore integration that has no sprint yet
// (placeholder lives at shithub-server/.docs/sprints/S54-supply-chain-attestations.md
// with the wire-shape contract this CLI skeleton expects).
//
// Every subcommand is a stub that prints a friendly notice and exits 2.
// We deliberately don't pull in sigstore-go yet — `attestation inspect`
// could in principle be local-only and ship today, but doing so before
// the server side commits to a Sigstore stack would lock the CLI to a
// vendor choice the server may not match.  See .docs/sprints/C23-attestation.md.
package attestation

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

const deferredMessage = "attestations not yet supported on this shithub host (see shithub-S54); subcommand registered for future use"

const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder. Same pattern as pkg/cmd/{release,gist,project,ruleset}.
var exitFn = os.Exit

func runDeferred(cmd *cobra.Command, _ []string) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
	exitFn(deferredExitCode)
	return nil
}

// NewCmd builds the `attestation` parent and attaches the four planned
// subcommands as stubs.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attestation <command>",
		Short: "Verify and inspect Sigstore artifact attestations (deferred until Sigstore integration ships)",
		Long: `Verify and inspect Sigstore-style artifact attestations on shithub.

shithub has no Sigstore / Fulcio / transparency-log integration yet.
This command tree is registered today so the planned flag surface is
discoverable, but every invocation exits 2 with a friendly notice until
the supply-chain story lands server-side (placeholder spec: S54).

Building attestations is intentionally out of scope for the CLI —
provenance generation belongs in the CI runner / Actions workflow.`,
	}
	cmd.AddCommand(newStub("verify", "Verify an artifact against an attestation bundle (deferred)"))
	cmd.AddCommand(newStub("download", "Download attestation bundles for an artifact (deferred)"))
	cmd.AddCommand(newStub("inspect", "Print the decoded contents of a local attestation bundle (deferred)"))
	cmd.AddCommand(newStub("trusted-root", "Fetch the Sigstore TUF trusted-root for offline verification (deferred)"))
	return cmd
}

// newStub builds one deferred subcommand. No flags — locking the
// gh-compatible flag surface (--bundle, --cert-identity, etc.) before
// the server-side bundle shape is decided would just mislead users.
func newStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE:  runDeferred,
	}
}
