// SPDX-License-Identifier: AGPL-3.0-or-later

// Package commits implements `shithub search commits`.
package commits

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

	Author        string
	Committer     string
	AuthorName    string
	AuthorEmail   string
	CommitterName string
	Hash          string
	Parent        string
	Tree          string
	Owner         string
	Repo          string
	AuthorDate    string
	CommitterDate string
	Merge         bool
	NoMerge       bool

	Common   searchshared.CommonFlags
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
		Use:   "commits [<query>]",
		Short: "Search commits",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts.Query = strings.TrimSpace(strings.Join(args, " "))
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVar(&opts.Author, "author", "", "filter by author login")
	cmd.Flags().StringVar(&opts.Committer, "committer", "", "filter by committer login")
	cmd.Flags().StringVar(&opts.AuthorName, "author-name", "", "filter by author display name")
	cmd.Flags().StringVar(&opts.AuthorEmail, "author-email", "", "filter by author email")
	cmd.Flags().StringVar(&opts.CommitterName, "committer-name", "", "filter by committer display name")
	cmd.Flags().StringVar(&opts.Hash, "hash", "", "filter by commit hash prefix")
	cmd.Flags().StringVar(&opts.Parent, "parent", "", "filter by parent commit hash")
	cmd.Flags().StringVar(&opts.Tree, "tree", "", "filter by tree SHA")
	cmd.Flags().StringVar(&opts.Owner, "owner", "", "restrict to user or org")
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "restrict to OWNER/NAME")
	cmd.Flags().StringVar(&opts.AuthorDate, "author-date", "", "filter on author date (YYYY-MM-DD or range)")
	cmd.Flags().StringVar(&opts.CommitterDate, "committer-date", "", "filter on committer date (range)")
	cmd.Flags().BoolVar(&opts.Merge, "merge", false, "include only merge commits")
	cmd.Flags().BoolVar(&opts.NoMerge, "no-merge", false, "omit merge commits")
	searchshared.AddCommonFlags(
		cmd, &opts.Common,
		"sort field: {author-date|committer-date|best-match}",
		"sort order: {asc|desc}",
	)
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the search.
func Run(ctx context.Context, opts *options) error {
	if opts.Merge && opts.NoMerge {
		return fmt.Errorf("search commits: --merge and --no-merge are mutually exclusive")
	}
	quals := []searchshared.Qualifier{
		{Key: "author", Value: opts.Author},
		{Key: "committer", Value: opts.Committer},
		{Key: "author-name", Value: opts.AuthorName},
		{Key: "author-email", Value: opts.AuthorEmail},
		{Key: "committer-name", Value: opts.CommitterName},
		{Key: "hash", Value: opts.Hash},
		{Key: "parent", Value: opts.Parent},
		{Key: "tree", Value: opts.Tree},
		{Key: "user", Value: opts.Owner},
		{Key: "repo", Value: opts.Repo},
		{Key: "author-date", Value: opts.AuthorDate},
		{Key: "committer-date", Value: opts.CommitterDate},
	}
	switch {
	case opts.Merge:
		quals = append(quals, searchshared.Qualifier{Key: "merge", Value: "true"})
	case opts.NoMerge:
		quals = append(quals, searchshared.Qualifier{Key: "merge", Value: "false"})
	}
	query := searchshared.ComposeQuery(opts.Query, quals...)
	if query == "" {
		return fmt.Errorf("search commits: provide a query string or at least one filter flag")
	}

	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if opts.Common.Web {
		url := searchshared.WebSearchURL(host, "commits", query)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	sc := search.NewClient(client)
	resp, err := sc.Commits(ctx, query, opts.Common.ToOptions())
	if err != nil {
		return err
	}
	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, resp.Items, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, resp)
	return nil
}

func render(io *iostreams.IOStreams, resp *search.Response[search.CommitItem]) {
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
		date := ""
		if it.Commit.Author != nil {
			date = it.Commit.Author.Date.Format("2006-01-02")
		}
		tp.AddRow(repo, shortSHA(it.SHA), date, firstLine(it.Commit.Message))
	}
	_ = tp.Render()
	if resp.IncompleteResults {
		fmt.Fprintln(io.ErrOut, "warning: server reported incomplete results — try narrowing the query")
	}
}

func shortSHA(s string) string {
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 80 {
		s = s[:79] + "…"
	}
	return s
}
