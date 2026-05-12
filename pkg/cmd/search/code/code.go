// SPDX-License-Identifier: AGPL-3.0-or-later

// Package code implements `shithub search code`. Renders matching
// files with optional snippet previews from the server's ts_headline.
package code

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

	Filename  string
	Extension string
	Language  string
	Owner     string
	Repo      string
	Size      string
	Match     string

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
		Use:   "code [<query>]",
		Short: "Search code in repositories",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			opts.Query = strings.TrimSpace(strings.Join(args, " "))
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVar(&opts.Filename, "filename", "", "restrict to a specific filename")
	cmd.Flags().StringVar(&opts.Extension, "extension", "", "restrict by file extension")
	cmd.Flags().StringVar(&opts.Language, "language", "", "restrict by detected language")
	cmd.Flags().StringVar(&opts.Owner, "owner", "", "restrict to user or org")
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "restrict to OWNER/NAME")
	cmd.Flags().StringVar(&opts.Size, "size", "", "filter on file size in bytes (range)")
	cmd.Flags().StringVar(&opts.Match, "match", "", "restrict free-text match to: {file|path}")
	searchshared.AddCommonFlags(
		cmd, &opts.Common,
		"sort field: {indexed|best-match}",
		"sort order: {asc|desc}",
	)
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the search.
func Run(ctx context.Context, opts *options) error {
	quals := []searchshared.Qualifier{
		{Key: "filename", Value: opts.Filename},
		{Key: "extension", Value: opts.Extension},
		{Key: "language", Value: opts.Language},
		{Key: "user", Value: opts.Owner},
		{Key: "repo", Value: opts.Repo},
		{Key: "in", Value: opts.Match},
	}
	if opts.Size != "" {
		r, err := searchshared.ParseRange(opts.Size)
		if err != nil {
			return fmt.Errorf("search code: --size: %w", err)
		}
		quals = append(quals, searchshared.Qualifier{Key: "size", Value: r.String()})
	}
	query := searchshared.ComposeQuery(opts.Query, quals...)
	if query == "" {
		return fmt.Errorf("search code: provide a query string or at least one filter flag")
	}

	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if opts.Common.Web {
		url := searchshared.WebSearchURL(host, "code", query)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	sc := search.NewClient(client)
	resp, err := sc.Code(ctx, query, opts.Common.ToOptions())
	if err != nil {
		return err
	}
	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, resp.Items, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, resp)
	return nil
}

// render emits one row per file with a one-line snippet pulled from
// the first TextMatch fragment when present.
func render(io *iostreams.IOStreams, resp *search.Response[search.CodeItem]) {
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
		tp.AddRow(repo, it.Path, snippet(it))
	}
	_ = tp.Render()
	if resp.IncompleteResults {
		fmt.Fprintln(io.ErrOut, "warning: server reported incomplete results — try narrowing the query")
	}
}

// snippet returns a single-line preview from the first text match,
// collapsing internal whitespace so the table doesn't tear.
func snippet(it search.CodeItem) string {
	if len(it.TextMatches) == 0 {
		return ""
	}
	f := strings.TrimSpace(it.TextMatches[0].Fragment)
	f = strings.ReplaceAll(f, "\n", " ")
	if len(f) > 80 {
		f = f[:79] + "…"
	}
	return f
}
