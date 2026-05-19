// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub issue list`. Lists issues for the
// resolved repo with filter / sort / limit flags, rendered as a table
// for TTY, TSV for non-TTY, or JSON via the standard --json pipeline.
package list

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// DefaultLimit mirrors gh's `issue list` default.
const DefaultLimit = 30

// MaxLimit caps `--limit` so a typo doesn't kick off a 50k-page traversal.
const MaxLimit = 1000

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner
	Opener      func(url string) error

	Repo     string
	Hostname string

	State     string
	Labels    []string
	Author    string
	Assignee  string
	Mention   string
	Milestone string
	Search    string
	Limit     int
	Web       bool

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Opener:      defaultOpener,
		State:       "open",
		Limit:       DefaultLimit,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List issues in a repository",
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
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "filter by state: {open|closed|all}")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "filter by label (repeatable, CSV)")
	cmd.Flags().StringVar(&opts.Author, "author", "", "filter by author (@me supported)")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "filter by assignee (@me supported)")
	cmd.Flags().StringVar(&opts.Mention, "mention", "", "filter by mention (@me supported)")
	cmd.Flags().StringVarP(&opts.Milestone, "milestone", "m", "", "filter by milestone number or title")
	cmd.Flags().StringVarP(&opts.Search, "search", "S", "", "search query (passes through to /search/issues; placeholder)")
	// G12 (F41): the --search flag is declared but not wired into the
	// list path. Hide from --help until C13 reaches the list endpoint;
	// the flag remains valid on the command line (back-compat with
	// scripts that pass it) but no longer self-advertises.
	_ = cmd.Flags().MarkHidden("search")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", DefaultLimit, "maximum number of issues to fetch")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the issues list in a browser")
	output.AddFlags(cmd, &opts.Exporter)
	output.MarkWebMutuallyExclusive(cmd)
	return cmd
}

// Run executes the list operation.
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

	if opts.Web {
		url := repocmdshared.WebURL(ref) + "/issues"
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	assignee, err := issueshared.ExpandMeSingle(ctx, client, opts.Assignee)
	if err != nil {
		return err
	}
	author, err := issueshared.ExpandMeSingle(ctx, client, opts.Author)
	if err != nil {
		return err
	}
	mention, err := issueshared.ExpandMeSingle(ctx, client, opts.Mention)
	if err != nil {
		return err
	}

	ic := issues.NewClient(client)
	listOpts := issues.ListOptions{
		State:     opts.State,
		Labels:    issueshared.SplitList(opts.Labels),
		Author:    author,
		Assignee:  assignee,
		Mentioned: mention,
		Milestone: opts.Milestone,
		Limit:     opts.Limit,
	}
	all, err := ic.List(ctx, ref.Owner, ref.Name, listOpts)
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, all, opts.IO.IsStdoutTTY())
	}
	renderTable(opts.IO, all)
	return nil
}

// renderTable emits a table view of the issue listing. Columns chosen to
// match gh: number, state, title, labels, age.
func renderTable(io *iostreams.IOStreams, list []issues.Issue) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no issues found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, is := range list {
		tp.AddRow(
			fmt.Sprintf("#%d", is.Number),
			is.State,
			truncate(is.Title, 70),
			labelsString(is.Labels),
			humanAge(is.UpdatedAt),
		)
	}
	_ = tp.Render()
}

// labelsString collapses the labels slice into a comma-separated cell.
func labelsString(ls []issues.Label) string {
	parts := make([]string, 0, len(ls))
	for _, l := range ls {
		parts = append(parts, l.Name)
	}
	return strings.Join(parts, ",")
}

// truncate clips s to n runes with an ellipsis. Keeps long titles from
// blowing out the table width.
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

// humanAge formats t as gh's relative-age string ("2d ago"). Deliberately
// coarse — hours/days/weeks/months/years — because issue lists care more
// about staleness than precision.
func humanAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	}
	return fmt.Sprintf("%dy ago", int(d.Hours()/(24*365)))
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

// defaultOpener is overridden in tests; production shells out to the
// platform's URL opener.
var defaultOpener = func(_ string) error { return nil }
