// SPDX-License-Identifier: AGPL-3.0-or-later

// Package view implements `shithub repo view`. The command renders a
// repo summary (header, stats, description, topics, README) for TTY use
// and exposes --json for scripts. --web swaps the render for a browser
// open.
package view

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// options captures the flag + arg state for a single invocation.
type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	RepoArg  string
	Repo     string
	Hostname string
	Web      bool
	Branch   string

	Exporter output.Options

	// Opener is the function called when --web is in effect. Override in tests.
	Opener func(url string) error
}

// NewCmd builds the cobra command. Wired by pkg/cmd/repo/repo.NewCmd.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Opener:      openInBrowser,
	}
	cmd := &cobra.Command{
		Use:   "view [<owner>/<repo>]",
		Short: "View a repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if opts.GitRunner == nil {
				// Resolution falls back to git remote when the user didn't
				// pass an arg or -R. A missing git binary is non-fatal —
				// Resolver tolerates a nil GitRunner.
				if r, err := git.FromPath(); err == nil {
					opts.GitRunner = r
				}
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the repository in the browser")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "branch to view the README from (default: default branch)")
	output.AddFlags(cmd, &opts.Exporter)
	output.MarkWebMutuallyExclusive(cmd)
	return cmd
}

// Run executes the view operation. Public so command tests can drive the
// behavior directly without going through cobra.
func Run(ctx context.Context, opts *options) error {
	if opts.RepoArg != "" && opts.Repo != "" {
		return fmt.Errorf("repo view: pass either a positional arg or -R, not both")
	}
	flagOrArg := opts.Repo
	if flagOrArg == "" {
		flagOrArg = opts.RepoArg
	}
	resolver := shared.Resolver{
		RepoFlag:    flagOrArg,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	ref, err := resolver.Resolve()
	if err != nil {
		return err
	}

	if opts.Web {
		url := shared.WebURL(ref)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	rc := repos.NewClient(client)

	repo, err := rc.View(ctx, ref.Owner, ref.Name)
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, repo, opts.IO.IsStdoutTTY())
	}

	renderHuman(opts.IO, repo)

	branch := opts.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}
	_, content, err := rc.ReadREADME(ctx, ref.Owner, ref.Name, branch)
	switch {
	case err == nil && len(content) > 0:
		writeREADME(opts.IO, content)
	case err != nil:
		// README is optional; an error fetching it is informational, not fatal.
		fmt.Fprintf(opts.IO.ErrOut, "no readme available: %v\n", err)
	}
	return nil
}

// renderHuman writes the header/stats block used by TTY mode. The blocks
// are intentionally small text — no fancy box drawing — so the output
// renders consistently across terminal widths.
func renderHuman(io *iostreams.IOStreams, r *repos.Repo) {
	out := io.Out
	fmt.Fprintln(out, r.FullName)
	badges := badgeLine(r)
	if badges != "" {
		fmt.Fprintln(out, badges)
	}
	if r.Description != "" {
		fmt.Fprintln(out, r.Description)
	}
	fmt.Fprintf(out, "★ %d  ⑂ %d  watching %d\n", r.Stargazers, r.Forks, r.Watchers)
	fmt.Fprintf(out, "default branch: %s\n", r.DefaultBranch)
	if len(r.Topics) > 0 {
		fmt.Fprintln(out, "topics:", strings.Join(r.Topics, ", "))
	}
	if r.HTMLURL != "" {
		fmt.Fprintln(out, r.HTMLURL)
	}
	fmt.Fprintln(out)
}

// badgeLine renders the visibility/state markers (public/private,
// archived, fork). Order matches gh's typical layout.
func badgeLine(r *repos.Repo) string {
	tags := []string{}
	if r.Private {
		tags = append(tags, "private")
	} else if r.Visibility != "" {
		tags = append(tags, r.Visibility)
	} else {
		tags = append(tags, "public")
	}
	if r.Archived {
		tags = append(tags, "archived")
	}
	if r.Fork {
		tags = append(tags, "fork")
	}
	return strings.Join(tags, " · ")
}

// writeREADME tries to render content as markdown via glamour; on any
// error it falls back to plain bytes so a misformatted README still shows.
func writeREADME(io *iostreams.IOStreams, content []byte) {
	rendered, err := io.RenderMarkdown(string(content))
	if err != nil {
		_, _ = io.Out.Write(content)
		return
	}
	_, _ = io.Out.Write([]byte(rendered))
}

// hostOrDefault unwraps the factory's DefaultHost func, treating nil as
// "let resolver use the package-level default".
func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

// openInBrowser shells out to the platform's URL opener. Kept tiny so we
// don't take a dep on a browser package for one command.
func openInBrowser(url string) error {
	bin, args := osOpenArgs(url)
	if bin == "" {
		return fmt.Errorf("repo view: no browser opener available for this platform")
	}
	return runCommand(bin, args, io.Discard, io.Discard)
}
