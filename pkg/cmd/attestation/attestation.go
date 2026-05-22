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
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewCmd builds the `attestation` parent. H4 migration to shared
// cmdutil.NewDeferredCmd factory.
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
	stub := func(name, short string) *cobra.Command {
		return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
			Use: name, Short: short,
			Name: "shithub attestation " + name, Track: "shithub-S54",
		})
	}
	cmd.AddCommand(stub("verify", "Verify an artifact against an attestation bundle (deferred)"))
	cmd.AddCommand(stub("download", "Download attestation bundles for an artifact (deferred)"))
	cmd.AddCommand(stub("inspect", "Print the decoded contents of a local attestation bundle (deferred)"))
	cmd.AddCommand(stub("trusted-root", "Fetch the Sigstore TUF trusted-root for offline verification (deferred)"))
	return cmd
}
