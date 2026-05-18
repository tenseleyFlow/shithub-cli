// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub label list`.
package list

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/labels"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// DefaultLimit matches gh's `label list` default.
const DefaultLimit = 30

// MaxLimit caps `--limit` to a sane upper bound.
const MaxLimit = 1000

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Repo     string
	Hostname string

	Search    string
	Sort      string
	Direction string
	Limit     int

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Sort:        "name",
		Direction:   "asc",
		Limit:       DefaultLimit,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List repository labels",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if opts.GitRunner == nil {
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVarP(&opts.Search, "search", "S", "", "filter by substring match against name/description")
	cmd.Flags().StringVar(&opts.Sort, "sort", "name", "sort field: {name|created}")
	cmd.Flags().StringVar(&opts.Direction, "order", "asc", "sort order: {asc|desc}")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", DefaultLimit, "maximum number of labels to fetch")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the listing.
func Run(ctx context.Context, opts *options) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}
	if opts.Limit > MaxLimit {
		opts.Limit = MaxLimit
	}
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	ref, err := resolver.Resolve()
	if err != nil {
		return err
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	lc := labels.NewClient(client)

	all, err := lc.List(ctx, ref.Owner, ref.Name, labels.ListOptions{
		Sort:      opts.Sort,
		Direction: opts.Direction,
		Limit:     opts.Limit,
	})
	if err != nil {
		return err
	}
	filtered := filter(all, opts.Search)

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, filtered, opts.IO.IsStdoutTTY())
	}
	renderTable(opts.IO, filtered)
	return nil
}

// filter narrows the list to labels whose name or description matches
// the --search query (case-insensitive substring).
func filter(in []labels.Label, search string) []labels.Label {
	if search == "" {
		return in
	}
	needle := strings.ToLower(search)
	out := make([]labels.Label, 0, len(in))
	for _, l := range in {
		if strings.Contains(strings.ToLower(l.Name), needle) ||
			strings.Contains(strings.ToLower(l.Description), needle) {
			out = append(out, l)
		}
	}
	return out
}

// renderTable emits a TTY-friendly table; non-TTY falls through the
// tableprinter's TSV branch.
func renderTable(io *iostreams.IOStreams, list []labels.Label) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no labels found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, l := range list {
		tp.AddRow(l.Name, "#"+l.Color, truncate(l.Description, 70))
	}
	_ = tp.Render()
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

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
