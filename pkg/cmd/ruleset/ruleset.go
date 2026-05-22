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
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewCmd builds the `ruleset` parent and attaches the four planned
// subcommands as stubs.
//
// H4 (H22/H23): migrated from a per-package newStub to the shared
// cmdutil.NewDeferredCmd factory so each stub accepts -R / --hostname
// / --json / --jq / --template uniformly. Pre-fix `ruleset list -R foo`
// errored at flag parsing with "unknown shorthand flag: 'R'" before
// the deferred notice could surface.
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
	stub := func(name, short string) *cobra.Command {
		return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
			Use: name, Short: short,
			Name: "shithub ruleset " + name, Track: "shithub-S20",
		})
	}
	cmd.AddCommand(stub("list", "List rulesets for a repository (deferred)"))
	cmd.AddCommand(stub("view", "Show a ruleset's full configuration (deferred)"))
	cmd.AddCommand(stub("check", "Show which rulesets apply to a given branch (deferred)"))
	cmd.AddCommand(stub("history", "Show the edit history of a ruleset (deferred)"))
	return cmd
}
