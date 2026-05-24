// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub pr list`. Filters + sort + limit
// against /pulls; table for TTY / TSV non-TTY / JSON via --json.
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
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// DefaultLimit mirrors gh's `pr list` default.
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

	State    string
	Labels   []string
	Author   string
	Assignee string
	Base     string
	Head     string
	Draft    string // "true" | "false" | "" (unset)
	Search   string
	Limit    int
	Web      bool

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Opener:      func(_ string) error { return nil },
		State:       "open",
		Limit:       DefaultLimit,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests in a repository",
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
	cmd.Flags().StringVarP(&opts.State, "state", "s", "open", "filter by state: {open|closed|merged|all}")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "filter by label (repeatable, CSV)")
	cmd.Flags().StringVar(&opts.Author, "author", "", "filter by author (@me supported)")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "filter by assignee (@me supported)")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "filter by base branch")
	cmd.Flags().StringVarP(&opts.Head, "head", "H", "", "filter by head branch (or user:branch)")
	cmd.Flags().StringVar(&opts.Draft, "draft", "", "filter by draft status: {true|false}")
	cmd.Flags().StringVarP(&opts.Search, "search", "S", "", "search query (placeholder; lands with C13 search)")
	// G12 (F41): same as issue list — the flag exists but isn't wired
	// in the list path. Hide from --help until C13 reaches list.
	_ = cmd.Flags().MarkHidden("search")
	cmdutil.AddLimitFlag(cmd, &opts.Limit, DefaultLimit, "maximum number of PRs to fetch")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the PR list in a browser")
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
		url := repocmdshared.WebURL(ref) + "/pulls"
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	author, err := issueshared.ExpandMeSingle(ctx, client, opts.Author)
	if err != nil {
		return err
	}
	assignee, err := issueshared.ExpandMeSingle(ctx, client, opts.Assignee)
	if err != nil {
		return err
	}

	listOpts := pulls.ListOptions{
		State:    opts.State,
		Labels:   issueshared.SplitList(opts.Labels),
		Author:   author,
		Assignee: assignee,
		Base:     opts.Base,
		Head:     opts.Head,
		Limit:    opts.Limit,
	}
	if d, ok := parseDraftFlag(opts.Draft); ok {
		listOpts.Draft = &d
	}

	pc := pulls.NewClient(client)
	all, err := pc.List(ctx, ref.Owner, ref.Name, listOpts)
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, all, opts.IO.IsStdoutTTY())
	}
	renderTable(opts.IO, all)
	return nil
}

// parseDraftFlag accepts the small bool-ish vocabulary common to gh
// flag values. Anything else returns (_, false) so the filter is skipped.
func parseDraftFlag(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1":
		return true, true
	case "false", "no", "0":
		return false, true
	}
	return false, false
}

// renderTable emits the columns: number, state, title, branch, age.
func renderTable(io *iostreams.IOStreams, list []pulls.PR) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no pull requests found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, p := range list {
		// audit-I7: shared precedence (merged > closed > draft > open)
		// so closed-draft PRs no longer falsely render as "draft".
		state := prshared.DisplayState(&p)
		tp.AddRow(
			fmt.Sprintf("#%d", p.Number),
			state,
			truncate(p.Title, 70),
			p.Head.Ref,
			humanAge(p.UpdatedAt),
		)
	}
	_ = tp.Render()
}

// humanAge formats t as a coarse relative-age string.
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
