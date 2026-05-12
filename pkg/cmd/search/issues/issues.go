// SPDX-License-Identifier: AGPL-3.0-or-later

// Package issues implements `shithub search issues`. Hits /search/issues
// with `type=issue` so PRs are filtered out server-side.
package issues

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
	searchshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	Opener      func(url string) error

	Hostname string
	Query    string

	Issue  searchshared.IssueFlags
	Common searchshared.CommonFlags

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory, opener func(string) error) *cobra.Command {
	if opener == nil {
		opener = func(_ string) error { return nil }
	}
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Opener:      opener,
	}
	cmd := &cobra.Command{
		Use:   "issues [<query>]",
		Short: "Search for issues",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts.Query = strings.TrimSpace(strings.Join(args, " "))
			return Run(c.Context(), opts)
		},
	}
	bindIssueFlags(cmd, &opts.Issue)
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	searchshared.AddCommonFlags(
		cmd, &opts.Common,
		"sort field: {comments|reactions|created|updated|best-match}",
		"sort order: {asc|desc}",
	)
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// bindIssueFlags lives here (not in shared) so the PR subcommand can
// reuse the same binding routine — its only addition is the PR-extra
// flags which it binds on its own command separately.
func bindIssueFlags(cmd *cobra.Command, f *searchshared.IssueFlags) {
	cmd.Flags().StringVar(&f.State, "state", "", "filter by state: {open|closed}")
	cmd.Flags().StringVar(&f.Author, "author", "", "filter by author login")
	cmd.Flags().StringVar(&f.Assignee, "assignee", "", "filter by assignee login")
	cmd.Flags().StringSliceVar(&f.Label, "label", nil, "filter by label (repeatable)")
	cmd.Flags().StringVar(&f.Milestone, "milestone", "", "filter by milestone title")
	cmd.Flags().StringVar(&f.Mentions, "mentions", "", "filter by mentioned user")
	cmd.Flags().StringVar(&f.Involves, "involves", "", "filter by user involvement")
	cmd.Flags().StringVar(&f.Commenter, "commenter", "", "filter by commenter login")
	cmd.Flags().StringVar(&f.Repo, "repo", "", "restrict to OWNER/NAME")
	cmd.Flags().StringVar(&f.Owner, "owner", "", "restrict to user/org")
	cmd.Flags().StringVar(&f.Language, "language", "", "restrict by repository language")
	cmd.Flags().BoolVar(&f.Archived, "archived", false, "include only archived repositories")
	cmd.Flags().BoolVar(&f.NoArchived, "no-archived", false, "omit archived repositories")
	cmd.Flags().BoolVar(&f.Locked, "locked", false, "include only locked conversations")
	cmd.Flags().BoolVar(&f.NoLocked, "no-locked", false, "omit locked conversations")
	cmd.Flags().BoolVar(&f.NoComments, "no-comments", false, "include only issues with zero comments")
	cmd.Flags().StringVar(&f.Reactions, "reactions", "", "filter by reactions count (range)")
	cmd.Flags().StringVar(&f.Match, "match", "", "restrict free-text match to: {title|body|comments}")
}

// Run executes the search.
func Run(ctx context.Context, opts *options) error {
	if opts.Issue.Archived && opts.Issue.NoArchived {
		return fmt.Errorf("search issues: --archived and --no-archived are mutually exclusive")
	}
	if opts.Issue.Locked && opts.Issue.NoLocked {
		return fmt.Errorf("search issues: --locked and --no-locked are mutually exclusive")
	}
	quals, err := searchshared.BuildIssueQualifiers(opts.Issue)
	if err != nil {
		return fmt.Errorf("search issues: %w", err)
	}
	query := searchshared.ComposeQuery(opts.Query, quals...)
	if query == "" {
		return fmt.Errorf("search issues: provide a query string or at least one filter flag")
	}

	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if opts.Common.Web {
		url := searchshared.WebSearchURL(host, "issues", query)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	sc := search.NewClient(client)
	resp, err := sc.Issues(ctx, query, opts.Common.ToOptions())
	if err != nil {
		return err
	}
	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, resp.Items, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, resp)
	return nil
}

func render(io *iostreams.IOStreams, resp *search.Response[search.IssueItem]) {
	if len(resp.Items) == 0 {
		fmt.Fprintln(io.ErrOut, "no results found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, it := range resp.Items {
		repo := ""
		if it.Repository != nil {
			repo = it.Repository.FullName
		}
		tp.AddRow(
			repo,
			fmt.Sprintf("#%d", it.Number),
			it.State,
			truncate(it.Title, 80),
			it.UpdatedAt.Format("2006-01-02"),
		)
	}
	_ = tp.Render()
	if resp.IncompleteResults {
		fmt.Fprintln(io.ErrOut, "warning: server reported incomplete results — try narrowing the query")
	}
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// BindIssueFlags exposes the issue flag-binding routine for the `prs`
// subcommand. Keeping it exported lets the PR command share the exact
// same flag set without duplicating cobra wiring.
func BindIssueFlags(cmd *cobra.Command, f *searchshared.IssueFlags) {
	bindIssueFlags(cmd, f)
}
