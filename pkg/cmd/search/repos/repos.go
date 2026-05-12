// SPDX-License-Identifier: AGPL-3.0-or-later

// Package repos implements `shithub search repos`. Renders matching
// repositories as a table (TTY) or TSV / JSON. The full-text query
// string is passed verbatim to the server; flag-driven filters lower
// into appended qualifiers via the shared composer.
package repos

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

	Owner      string
	Language   string
	License    string
	Match      string
	Topic      []string
	Visibility string

	Archived   bool
	NoArchived bool
	Forks      string
	Stars      string
	HelpWanted string
	GoodFirst  string
	Size       string
	FollowedBy string

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
		Use:   "repos [<query>]",
		Short: "Search for repositories",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts.Query = strings.TrimSpace(strings.Join(args, " "))
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVar(&opts.Owner, "owner", "", "filter on owner (user or org)")
	cmd.Flags().StringVar(&opts.Language, "language", "", "filter by primary language")
	cmd.Flags().StringVar(&opts.License, "license", "", "filter by license SPDX id")
	cmd.Flags().StringVar(&opts.Match, "match", "", "restrict free-text match to one of: name, description, readme")
	cmd.Flags().StringSliceVar(&opts.Topic, "topic", nil, "filter by topic (repeatable)")
	cmd.Flags().StringVar(&opts.Visibility, "visibility", "", "filter by visibility: {public|private|internal}")
	cmd.Flags().BoolVar(&opts.Archived, "archived", false, "show only archived repositories")
	cmd.Flags().BoolVar(&opts.NoArchived, "no-archived", false, "omit archived repositories")
	cmd.Flags().StringVar(&opts.Forks, "forks", "", "filter on forks count (range: >N, <N, N..M, *..N, N..*)")
	cmd.Flags().StringVar(&opts.Stars, "stars", "", "filter on stargazers count (range)")
	cmd.Flags().StringVar(&opts.HelpWanted, "help-wanted-issues", "", "filter on help-wanted issue count (range)")
	cmd.Flags().StringVar(&opts.GoodFirst, "good-first-issues", "", "filter on good-first-issue count (range)")
	cmd.Flags().StringVar(&opts.Size, "size", "", "filter on size in KB (range)")
	cmd.Flags().StringVar(&opts.FollowedBy, "followed-by", "", "filter to repos a user follows")
	searchshared.AddCommonFlags(
		cmd, &opts.Common,
		"sort field: {forks|help-wanted-issues|stars|updated|best-match}",
		"sort order: {asc|desc}",
	)
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the search.
//
//nolint:gocyclo // flag → qualifier lowering is unavoidably long
func Run(ctx context.Context, opts *options) error {
	if opts.Archived && opts.NoArchived {
		return fmt.Errorf("search repos: --archived and --no-archived are mutually exclusive")
	}

	quals, err := buildQualifiers(opts)
	if err != nil {
		return err
	}
	query := searchshared.ComposeQuery(opts.Query, quals...)
	if query == "" {
		return fmt.Errorf("search repos: provide a query string or at least one filter flag")
	}

	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}

	if opts.Common.Web {
		url := searchshared.WebSearchURL(host, "repositories", query)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	sc := search.NewClient(client)

	resp, err := sc.Repositories(ctx, query, opts.Common.ToOptions())
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, resp.Items, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, resp)
	return nil
}

// buildQualifiers lowers the structured flags into the search package's
// `key:value` form. The full-text portion of the query — `opts.Query` —
// is layered on by ComposeQuery later so users can freely mix the two.
func buildQualifiers(opts *options) ([]searchshared.Qualifier, error) {
	out := []searchshared.Qualifier{
		{Key: "user", Value: opts.Owner},
		{Key: "language", Value: opts.Language},
		{Key: "license", Value: opts.License},
		{Key: "in", Value: opts.Match},
		{Key: "is", Value: opts.Visibility},
	}
	out = append(out, searchshared.QualifierAll("topic", opts.Topic)...)

	for _, spec := range []struct {
		key, val string
	}{
		{"forks", opts.Forks},
		{"stars", opts.Stars},
		{"help-wanted-issues", opts.HelpWanted},
		{"good-first-issues", opts.GoodFirst},
		{"size", opts.Size},
	} {
		if spec.val == "" {
			continue
		}
		r, err := searchshared.ParseRange(spec.val)
		if err != nil {
			return nil, fmt.Errorf("search repos: --%s: %w", spec.key, err)
		}
		out = append(out, searchshared.Qualifier{Key: spec.key, Value: r.String()})
	}
	switch {
	case opts.Archived:
		out = append(out, searchshared.Qualifier{Key: "archived", Value: "true"})
	case opts.NoArchived:
		out = append(out, searchshared.Qualifier{Key: "archived", Value: "false"})
	}
	if opts.FollowedBy != "" {
		out = append(out, searchshared.Qualifier{Key: "followed-by", Value: opts.FollowedBy})
	}
	return out, nil
}

// render writes a table summary to stdout. Empty result sets get an
// explanatory stderr message and a clean exit (scripts can rely on
// total_count via --json).
func render(io *iostreams.IOStreams, resp *search.Response[search.RepoItem]) {
	if len(resp.Items) == 0 {
		fmt.Fprintln(io.ErrOut, "no results found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, r := range resp.Items {
		tp.AddRow(
			r.FullName,
			truncate(r.Description, 80),
			fmt.Sprintf("★ %d", r.Stargazers),
			r.UpdatedAt.Format("2006-01-02"),
		)
	}
	_ = tp.Render()
	if resp.IncompleteResults {
		fmt.Fprintln(io.ErrOut, "warning: server reported incomplete results — try narrowing the query")
	}
}

// truncate clips s to n runes with an ellipsis tail.
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
