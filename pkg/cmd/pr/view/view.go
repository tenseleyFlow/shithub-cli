// SPDX-License-Identifier: AGPL-3.0-or-later

// Package view implements `shithub pr view`. With no arg, finds the PR
// for the current branch via branch-to-PR lookup; otherwise accepts a
// number, URL, or branch name. --web/--comments/--json behave gh-style.
package view

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/list"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner
	Opener      func(url string) error

	Arg      string
	Repo     string
	Hostname string
	Web      bool
	Comments bool

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Opener:      func(_ string) error { return nil },
	}
	cmd := &cobra.Command{
		Use:   "view [<number-or-url-or-branch>]",
		Short: "View a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
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
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the PR in a browser")
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "include the conversation thread inline")
	output.AddFlags(cmd, &opts.Exporter)
	output.MarkWebMutuallyExclusive(cmd)
	return cmd
}

// Run executes the view operation.
//
//nolint:gocyclo // dispatch over arg/no-arg/web/comments paths.
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, err := resolver.Resolve()
	if err != nil && (opts.Arg == "" || !strings.Contains(opts.Arg, "://")) {
		return err
	}
	if err != nil {
		fb = repocmdshared.RepoRef{}
	}

	client, err := opts.HTTPClient(fb.Host)
	if err != nil {
		return err
	}
	pc := pulls.NewClient(client)

	var ref prshared.PRRef
	if opts.Arg != "" {
		ref, err = prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
		if err != nil {
			return err
		}
	} else {
		// No arg → current branch.
		branch := prshared.CurrentBranchFromGit(opts.GitRunner, "")
		if branch == "" {
			return fmt.Errorf("pr view: pass a number/URL/branch (couldn't detect current branch)")
		}
		pr, err := prshared.FindPRByBranch(ctx, pc, fb, branch)
		if err != nil {
			return err
		}
		ref = prshared.PRRef{Repo: fb, Number: pr.Number}
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	if opts.Web {
		url := prshared.PRWebURL(ref)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	// Re-resolve client when the URL form pointed at a different host than fb.
	if ref.Repo.Host != fb.Host {
		client, err = opts.HTTPClient(ref.Repo.Host)
		if err != nil {
			return err
		}
		pc = pulls.NewClient(client)
	}

	pr, err := pc.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, pr, opts.IO.IsStdoutTTY())
	}

	renderHuman(opts.IO, ref.Repo, pr)

	if opts.Comments {
		ic := issues.NewClient(client)
		comments, cerr := ic.ListComments(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
		if cerr != nil {
			fmt.Fprintf(opts.IO.ErrOut, "failed to fetch comments: %v\n", cerr)
			return nil
		}
		renderComments(opts.IO, comments)
	}
	return nil
}

// renderHuman writes the TTY-friendly summary.
func renderHuman(io *iostreams.IOStreams, repo repocmdshared.RepoRef, p *pulls.PR) {
	out := io.Out
	state := p.State
	if p.Draft {
		state = "draft"
	}
	if p.Merged {
		state = "merged"
	}
	fmt.Fprintf(out, "%s#%d  %s\n", repo.FullName(), p.Number, p.Title)
	fmt.Fprintf(out, "%s · %s · base:%s ← head:%s\n", state, authorLogin(p), p.Base.Ref, p.Head.Ref)
	if len(p.Labels) > 0 {
		names := make([]string, 0, len(p.Labels))
		for _, l := range p.Labels {
			names = append(names, l.Name)
		}
		fmt.Fprintln(out, "labels:", strings.Join(names, ", "))
	}
	if len(p.Assignees) > 0 {
		names := make([]string, 0, len(p.Assignees))
		for _, a := range p.Assignees {
			names = append(names, a.Login)
		}
		fmt.Fprintln(out, "assignees:", strings.Join(names, ", "))
	}
	if len(p.RequestedRevs) > 0 {
		names := make([]string, 0, len(p.RequestedRevs))
		for _, a := range p.RequestedRevs {
			names = append(names, a.Login)
		}
		fmt.Fprintln(out, "reviewers:", strings.Join(names, ", "))
	}
	if p.ReviewDecision != "" {
		fmt.Fprintln(out, "review decision:", p.ReviewDecision)
	}
	if p.MergeableState != "" {
		fmt.Fprintln(out, "mergeable:", p.MergeableState)
	}
	if p.HTMLURL != "" {
		fmt.Fprintln(out, p.HTMLURL)
	}
	fmt.Fprintln(out)
	if p.Body != "" {
		writeMarkdown(io, p.Body)
	}
}

// renderComments writes each comment with a small separator.
func renderComments(io *iostreams.IOStreams, list []issues.Comment) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no comments")
		return
	}
	fmt.Fprintln(io.Out)
	for _, c := range list {
		author := "unknown"
		if c.User != nil {
			author = c.User.Login
		}
		fmt.Fprintf(io.Out, "--- %s · %s ---\n", author, c.CreatedAt.Format("2006-01-02 15:04"))
		writeMarkdown(io, c.Body)
		fmt.Fprintln(io.Out)
	}
}

// writeMarkdown renders content with glamour; on render error falls
// back to plain bytes. E-audit E24: always end with a newline so the
// body doesn't visually merge with the next shell prompt.
func writeMarkdown(io *iostreams.IOStreams, content string) {
	rendered, err := io.RenderMarkdown(content)
	if err != nil {
		rendered = content
	}
	fmt.Fprint(io.Out, rendered)
	if !strings.HasSuffix(rendered, "\n") {
		fmt.Fprintln(io.Out)
	}
}

func authorLogin(p *pulls.PR) string {
	if p.User == nil {
		return "ghost"
	}
	return p.User.Login
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

// exporter wraps list.ProjectPR for single-PR view's --json output.
type exporter struct{}

func (exporter) Fields() []string { return listExportableFields }

// listExportableFields mirrors the list package's field catalog so view
// and list have a single source of truth. Imported via the local var.
var listExportableFields = list.ExportableFields()

func (exporter) Filter(v any) (any, error) {
	p, ok := v.(*pulls.PR)
	if !ok {
		return nil, fmt.Errorf("pr view exporter: want *pulls.PR, got %T", v)
	}
	return list.ProjectPR(*p), nil
}
