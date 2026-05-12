// SPDX-License-Identifier: AGPL-3.0-or-later

// Package prs implements `shithub search prs`. Wraps /search/issues
// with `type=pr` so only pull requests come back, then layers PR-only
// filters (draft, merged, checks state, review decision, base/head/app)
// on top of the shared issue qualifier set.
package prs

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
	searchissues "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/issues"
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

	Draft           bool
	NoDraft         bool
	Merged          bool
	NoMerged        bool
	Checks          string
	Review          string
	ReviewRequested string
	ReviewedBy      string
	Base            string
	Head            string
	App             string

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
		Use:   "prs [<query>]",
		Short: "Search for pull requests",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts.Query = strings.TrimSpace(strings.Join(args, " "))
			return Run(c.Context(), opts)
		},
	}
	searchissues.BindIssueFlags(cmd, &opts.Issue)
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "include only draft PRs")
	cmd.Flags().BoolVar(&opts.NoDraft, "no-draft", false, "omit draft PRs")
	cmd.Flags().BoolVar(&opts.Merged, "merged", false, "include only merged PRs")
	cmd.Flags().BoolVar(&opts.NoMerged, "no-merged", false, "omit merged PRs")
	cmd.Flags().StringVar(&opts.Checks, "checks", "", "filter on checks: {pending|passing|failing}")
	cmd.Flags().StringVar(&opts.Review, "review", "", "filter on review status: {none|required|approved|changes_requested}")
	cmd.Flags().StringVar(&opts.ReviewRequested, "review-requested", "", "filter on review-requested user")
	cmd.Flags().StringVar(&opts.ReviewedBy, "reviewed-by", "", "filter on reviewer login")
	cmd.Flags().StringVar(&opts.Base, "base", "", "filter by base branch")
	cmd.Flags().StringVar(&opts.Head, "head", "", "filter by head branch")
	cmd.Flags().StringVar(&opts.App, "app", "", "filter by author app slug")
	searchshared.AddCommonFlags(
		cmd, &opts.Common,
		"sort field: {comments|reactions|created|updated|best-match}",
		"sort order: {asc|desc}",
	)
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the search.
//
//nolint:gocyclo // boolean-pair conflict checks + qualifier lowering
func Run(ctx context.Context, opts *options) error {
	if opts.Issue.Archived && opts.Issue.NoArchived {
		return fmt.Errorf("search prs: --archived and --no-archived are mutually exclusive")
	}
	if opts.Issue.Locked && opts.Issue.NoLocked {
		return fmt.Errorf("search prs: --locked and --no-locked are mutually exclusive")
	}
	if opts.Draft && opts.NoDraft {
		return fmt.Errorf("search prs: --draft and --no-draft are mutually exclusive")
	}
	if opts.Merged && opts.NoMerged {
		return fmt.Errorf("search prs: --merged and --no-merged are mutually exclusive")
	}

	quals, err := searchshared.BuildIssueQualifiers(opts.Issue)
	if err != nil {
		return fmt.Errorf("search prs: %w", err)
	}
	quals = append(quals, prQualifiers(opts)...)

	query := searchshared.ComposeQuery(opts.Query, quals...)
	if query == "" {
		return fmt.Errorf("search prs: provide a query string or at least one filter flag")
	}

	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if opts.Common.Web {
		url := searchshared.WebSearchURL(host, "pullrequests", query)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	sc := search.NewClient(client)
	resp, err := sc.PullRequests(ctx, query, opts.Common.ToOptions())
	if err != nil {
		return err
	}
	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, resp.Items, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, resp)
	return nil
}

// prQualifiers lowers PR-only flags into the qualifier list. We use
// gh's `is:draft / -is:draft` and `is:merged / -is:merged` mapping —
// shithub's server parser mirrors gh.
func prQualifiers(opts *options) []searchshared.Qualifier {
	out := []searchshared.Qualifier{
		{Key: "status", Value: opts.Checks},
		{Key: "review", Value: opts.Review},
		{Key: "review-requested", Value: opts.ReviewRequested},
		{Key: "reviewed-by", Value: opts.ReviewedBy},
		{Key: "base", Value: opts.Base},
		{Key: "head", Value: opts.Head},
		{Key: "app", Value: opts.App},
	}
	switch {
	case opts.Draft:
		out = append(out, searchshared.Qualifier{Key: "is", Value: "draft"})
	case opts.NoDraft:
		out = append(out, searchshared.Qualifier{Key: "-is", Value: "draft"})
	}
	switch {
	case opts.Merged:
		out = append(out, searchshared.Qualifier{Key: "is", Value: "merged"})
	case opts.NoMerged:
		out = append(out, searchshared.Qualifier{Key: "-is", Value: "merged"})
	}
	return out
}

func render(io *iostreams.IOStreams, resp *search.Response[search.PRItem]) {
	if len(resp.Items) == 0 {
		fmt.Fprintln(io.ErrOut, "no results found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, pr := range resp.Items {
		state := pr.State
		if pr.Draft {
			state = "draft"
		}
		if pr.Merged {
			state = "merged"
		}
		tp.AddRow(
			fmt.Sprintf("#%d", pr.Number),
			state,
			truncate(pr.Title, 80),
			pr.Head.Ref,
			pr.UpdatedAt.Format("2006-01-02"),
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
