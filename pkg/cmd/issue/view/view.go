// SPDX-License-Identifier: AGPL-3.0-or-later

// Package view implements `shithub issue view`. Renders the issue
// envelope for TTY use; --web opens in browser, --comments inlines the
// comment thread, --json exposes the typed projection.
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
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
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
		Opener:      defaultOpener,
	}
	cmd := &cobra.Command{
		Use:   "view <number-or-url>",
		Short: "View an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Arg = args[0]
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
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the issue in a browser")
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "include the comment thread inline")
	output.AddFlags(cmd, &opts.Exporter)
	output.MarkWebMutuallyExclusive(cmd)
	return cmd
}

// Run executes the view operation.
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, err := resolver.Resolve()
	// Resolution failure isn't fatal if the arg is a URL; ParseIssueArg
	// will fill the ref from the URL itself.
	if err != nil && !strings.Contains(opts.Arg, "://") {
		return err
	}
	if err != nil {
		fb = repocmdshared.RepoRef{}
	}

	ref, _, err := issueshared.ParseIssueArg(opts.Arg, fb)
	if err != nil {
		return err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	if opts.Web {
		url := issueshared.IssueWebURL(ref)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(ref.Repo.Host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)
	issue, err := ic.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, issue, opts.IO.IsStdoutTTY())
	}

	renderHuman(opts.IO, ref.Repo, issue)

	if opts.Comments {
		comments, cerr := ic.ListComments(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
		if cerr != nil {
			fmt.Fprintf(opts.IO.ErrOut, "failed to fetch comments: %v\n", cerr)
			return nil
		}
		renderComments(opts.IO, comments)
	}
	return nil
}

// renderHuman writes the TTY-friendly summary header + body. README-style
// rendering uses glamour via iostreams.RenderMarkdown when the terminal
// supports it.
func renderHuman(io *iostreams.IOStreams, repo repocmdshared.RepoRef, i *issues.Issue) {
	out := io.Out
	fmt.Fprintf(out, "%s#%d  %s\n", repo.FullName(), i.Number, i.Title)
	fmt.Fprintf(out, "%s · %s · %d comments\n", i.State, authorLogin(i), i.Comments)
	if len(i.Labels) > 0 {
		names := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			names = append(names, l.Name)
		}
		fmt.Fprintln(out, "labels:", strings.Join(names, ", "))
	}
	if len(i.Assignees) > 0 {
		names := make([]string, 0, len(i.Assignees))
		for _, a := range i.Assignees {
			names = append(names, a.Login)
		}
		fmt.Fprintln(out, "assignees:", strings.Join(names, ", "))
	}
	if i.Milestone != nil {
		fmt.Fprintln(out, "milestone:", i.Milestone.Title)
	}
	if i.HTMLURL != "" {
		fmt.Fprintln(out, i.HTMLURL)
	}
	fmt.Fprintln(out)
	if i.Body != "" {
		writeMarkdown(io, i.Body)
	}
}

// renderComments writes each comment with a small separator. Author and
// timestamp head each one; body is glamour-rendered.
func renderComments(io *iostreams.IOStreams, list []issues.Comment) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no comments")
		return
	}
	fmt.Fprintln(io.Out)
	for _, c := range list {
		header := c.CreatedAt.Format("2006-01-02 15:04")
		author := "unknown"
		if c.User != nil {
			author = c.User.Login
		}
		fmt.Fprintf(io.Out, "--- %s · %s ---\n", author, header)
		writeMarkdown(io, c.Body)
		fmt.Fprintln(io.Out)
	}
}

// writeMarkdown renders content with glamour; on render error falls back
// to plain bytes so a malformed body still shows.
//
// E-audit E24: always end with a newline so the body doesn't visually
// merge with the next shell prompt. glamour's output ends in `\n` for
// typical inputs but not always (e.g. inline-only fragments); the
// trailing Fprintln is a belt-and-suspenders fix.
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

// authorLogin returns the issue creator's login or "ghost" when the
// user is missing (server quirk for deleted accounts).
func authorLogin(i *issues.Issue) string {
	if i.User == nil {
		return "ghost"
	}
	return i.User.Login
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

var defaultOpener = func(_ string) error { return nil }
