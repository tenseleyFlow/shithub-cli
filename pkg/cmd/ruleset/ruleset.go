// SPDX-License-Identifier: AGPL-3.0-or-later

// Package ruleset wires the `shithub ruleset` subtree onto the root
// command. shithub S20 ships the data model (branch_protection_rules
// table + HTML settings handlers) but no REST endpoints — the CLI
// can't surface protections until /api/v1/repos/{o}/{r}/rulesets etc.
// land. We register the verb tree as deferred stubs so users see the
// planned surface and `--help` discovery / completion work today.
//
// See .docs/sprints/C22-ruleset.md (CLI) and the "CLI integration
// notes" section in shithub-server/.docs/sprints/S20-*.md for the
// exact REST contract this skeleton expects.
package ruleset

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

const deferredMessage = "rulesets not yet exposed over REST on this shithub host (see shithub-S20 follow-up); subcommand registered for future use"

const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder. Same pattern as pkg/cmd/{release,gist,project}.
var exitFn = os.Exit

func runDeferred(cmd *cobra.Command, _ []string) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
	exitFn(deferredExitCode)
	return nil
}

// NewCmd builds the `ruleset` parent and attaches the four planned
// subcommands as stubs.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ruleset <command>",
		Short: "Inspect repository and org rulesets (deferred until REST API ships)",
		Long: `Inspect branch-protection rulesets on shithub.

shithub server S20 has the branch-protection data model (force-push
prevention, deletion prevention, required reviews, required checks) but
exposes it via HTML settings forms only. This command tree is registered
today so the planned flag surface is discoverable; every invocation
exits 2 with a friendly notice until the REST endpoints land.

Creation/deletion via CLI is intentionally out of scope (gh treats
ruleset config as web-only on GitHub — match).`,
	}
	cmd.AddCommand(newStub("list", "List rulesets for a repository (deferred)"))
	cmd.AddCommand(newStub("view", "Show a ruleset's full configuration (deferred)"))
	cmd.AddCommand(newStub("check", "Show which rulesets apply to a given branch (deferred)"))
	cmd.AddCommand(newStub("history", "Show the edit history of a ruleset (deferred)"))
	return cmd
}

// newStub builds one deferred subcommand. No flags — flag-shape
// compatibility isn't useful until the REST contract is fixed.
func newStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE:  runDeferred,
	}
}
