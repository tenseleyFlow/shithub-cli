// SPDX-License-Identifier: AGPL-3.0-or-later

// Package search wires the `shithub search` subtree onto the root
// command. Subcommands hit shithub's /api/v1/search/* surface (S50 §5)
// and share a common qualifier composer / range parser.
package search

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/browser"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	searchcode "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/code"
	searchcommits "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/commits"
	searchissues "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/issues"
	searchprs "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/prs"
	searchrepos "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/repos"
)

// NewCmd builds the `search` parent and attaches the five subcommands.
// `shithub search` with no subcommand prints help — matches gh, which
// also refuses to guess between repos/issues/code from a bare query.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <command>",
		Short: "Search across repos, issues, PRs, code, and commits",
		Long: `Search shithub for matching repos, issues, PRs, code, or commits.

Use the appropriate subcommand to disambiguate; bare-query search
matches gh's behavior and refuses to guess.`,
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(searchrepos.NewCmd(f, browser.Open))
	cmd.AddCommand(searchissues.NewCmd(f, browser.Open))
	cmd.AddCommand(searchprs.NewCmd(f, browser.Open))
	cmd.AddCommand(searchcode.NewCmd(f, browser.Open))
	cmd.AddCommand(searchcommits.NewCmd(f, browser.Open))
	return cmd
}
