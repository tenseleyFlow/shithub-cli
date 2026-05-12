// SPDX-License-Identifier: AGPL-3.0-or-later

// Package status implements `shithub pr status`. Cross-repo dashboard
// surfacing PRs created by the caller, review-requested from the caller,
// and mentioning the caller — plus a "current branch PR" pane when the
// working tree has one. Three queries against /issues with the
// is:pr-implicit endpoint scope; client filters PRs vs issues.
package status

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// SectionLimit caps each section so a flood in one category doesn't
// drown out the others. Total is 3 × SectionLimit + 1 (current-branch).
const SectionLimit = 10

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Repo     string
	Hostname string

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show pull requests relevant to you",
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
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the dashboard.
func Run(ctx context.Context, opts *options) error {
	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	created, err := ic.ListAcrossRepos(ctx, "created", issues.ListOptions{State: "open", Limit: SectionLimit})
	if err != nil {
		return fmt.Errorf("pr status: created: %w", err)
	}
	mentioned, err := ic.ListAcrossRepos(ctx, "mentioned", issues.ListOptions{State: "open", Limit: SectionLimit})
	if err != nil {
		return fmt.Errorf("pr status: mentioned: %w", err)
	}
	// Review-requested doesn't map onto /issues filters; use the same
	// endpoint with assigned-style filter as a stand-in until S50 §6
	// ships a dedicated query.
	requested, err := ic.ListAcrossRepos(ctx, "assigned", issues.ListOptions{State: "open", Limit: SectionLimit})
	if err != nil {
		return fmt.Errorf("pr status: review-requested: %w", err)
	}

	createdPRs := onlyPRs(created)
	mentionedPRs := onlyPRs(mentioned)
	requestedPRs := onlyPRs(requested)

	// Current-branch PR — best-effort.
	var current *pulls.PR
	if opts.GitRunner != nil {
		repoResolver := repocmdshared.Resolver{
			RepoFlag:    opts.Repo,
			Hostname:    opts.Hostname,
			DefaultHost: host,
			GitRunner:   opts.GitRunner,
		}
		if repo, err := repoResolver.Resolve(); err == nil {
			if branch := prshared.CurrentBranchFromGit(opts.GitRunner, ""); branch != "" {
				if pr, ferr := prshared.FindPRByBranch(ctx, pc, repo, branch); ferr == nil {
					current = pr
				}
			}
		}
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, map[string]any{
			"createdBy":       createdPRs,
			"mentioned":       mentionedPRs,
			"reviewRequested": requestedPRs,
			"currentBranch":   current,
		}, opts.IO.IsStdoutTTY())
	}

	if current != nil {
		fmt.Fprintln(opts.IO.Out, "Current branch")
		tp := tableprinter.New(opts.IO.Out, opts.IO.IsStdoutTTY(), opts.IO.TerminalWidth())
		tp.AddRow("  "+repoLabel(current), fmt.Sprintf("#%d", current.Number), truncate(current.Title, 70))
		_ = tp.Render()
		fmt.Fprintln(opts.IO.Out)
	}
	renderSection(opts.IO, "Created by you", createdPRs)
	renderSection(opts.IO, "Requesting a code review from you", requestedPRs)
	renderSection(opts.IO, "Mentioning you", mentionedPRs)
	return nil
}

// onlyPRs filters an issues-shaped list down to PR-flagged entries and
// converts to pulls.PR with the bare minimum fields the dashboard needs
// (number/title/repo/state/url). The shape is intentionally minimal —
// the dashboard isn't a PR detail view.
func onlyPRs(list []issues.Issue) []pulls.PR {
	out := make([]pulls.PR, 0, len(list))
	for _, i := range list {
		if !i.IsPullRequest() {
			continue
		}
		p := pulls.PR{
			Number:  i.Number,
			Title:   i.Title,
			State:   i.State,
			HTMLURL: i.HTMLURL,
		}
		if i.Repository != nil {
			p.Repository = i.Repository
		}
		out = append(out, p)
	}
	return out
}

// renderSection writes a subheader plus a small table.
func renderSection(io *iostreams.IOStreams, title string, list []pulls.PR) {
	fmt.Fprintln(io.Out, title)
	if len(list) == 0 {
		fmt.Fprintln(io.Out, "  nothing here")
		fmt.Fprintln(io.Out)
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, p := range list {
		tp.AddRow("  "+repoLabel(&p), fmt.Sprintf("#%d", p.Number), truncate(p.Title, 70))
	}
	_ = tp.Render()
	fmt.Fprintln(io.Out)
}

func repoLabel(p *pulls.PR) string {
	if p.Repository != nil {
		return p.Repository.FullName
	}
	return ""
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

// exporter is the JSON projection.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{"createdBy", "currentBranch", "mentioned", "reviewRequested"}
}

func (exporter) Filter(v any) (any, error) {
	b, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("pr status exporter: want map[string]any, got %T", v)
	}
	project := func(list []pulls.PR) []map[string]any {
		out := make([]map[string]any, 0, len(list))
		for _, p := range list {
			repo := ""
			if p.Repository != nil {
				repo = p.Repository.FullName
			}
			out = append(out, map[string]any{
				"number":     p.Number,
				"title":      p.Title,
				"state":      p.State,
				"url":        p.HTMLURL,
				"repository": repo,
			})
		}
		return out
	}
	out := map[string]any{
		"createdBy":       project(b["createdBy"].([]pulls.PR)),
		"mentioned":       project(b["mentioned"].([]pulls.PR)),
		"reviewRequested": project(b["reviewRequested"].([]pulls.PR)),
	}
	if cur, ok := b["currentBranch"].(*pulls.PR); ok && cur != nil {
		out["currentBranch"] = project([]pulls.PR{*cur})[0]
	}
	return out, nil
}
