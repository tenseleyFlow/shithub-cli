// SPDX-License-Identifier: AGPL-3.0-or-later

// Package browse implements `shithub browse`. Composes the shithub web
// URL for a target (repo home, PR, issue, file, commit, tab) and opens
// it in the user's browser. --no-browser prints the URL instead.
//
// Pure URL composition; no API calls (matches gh's design).
package browse

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/browser"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	DefaultHost func() string
	GitRunner   git.Runner
	Opener      func(url string) error

	Arg      string
	Repo     string
	Hostname string

	Branch    string
	Commit    string
	Projects  bool
	Releases  bool
	Settings  bool
	Wiki      bool
	NoBrowser bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		DefaultHost: f.DefaultHost,
		Opener:      browser.Open,
	}
	cmd := &cobra.Command{
		Use:   "browse [<target>]",
		Short: "Open the shithub repository in a browser",
		Long: `Open a shithub page in your browser.

Examples:
  shithub browse                           # repo home
  shithub browse 42                        # PR or issue #42
  shithub browse pr/42                     # PR #42 explicitly
  shithub browse README.md                 # file at default branch
  shithub browse main.go --branch dev      # file at branch dev
  shithub browse dev:src/main.go           # branch:path form
  shithub browse abc1234                   # commit view
  shithub browse --settings                # repo settings page
  shithub browse --no-browser 42           # print URL only
`,
		Args: cobra.MaximumNArgs(1),
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
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "branch to use when browsing a file or tree")
	cmd.Flags().StringVarP(&opts.Commit, "commit", "c", "", "commit SHA to view (alternative to positional arg)")
	cmd.Flags().BoolVarP(&opts.Projects, "projects", "p", false, "open the projects tab")
	cmd.Flags().BoolVarP(&opts.Releases, "releases", "r", false, "open the releases tab")
	cmd.Flags().BoolVarP(&opts.Settings, "settings", "s", false, "open the settings tab")
	cmd.Flags().BoolVarP(&opts.Wiki, "wiki", "w", false, "open the wiki tab")
	cmd.Flags().BoolVarP(&opts.NoBrowser, "no-browser", "n", false, "print the URL to stdout instead of opening it")
	return cmd
}

// Run executes the browse operation.
func Run(_ context.Context, opts *options) error {
	tab, err := pickTab(opts)
	if err != nil {
		return err
	}

	target, comp := Classify(opts.Arg)
	// --commit overrides positional when both present, mirroring gh.
	if opts.Commit != "" {
		if target != TargetNone {
			return errors.New("browse: --commit and a positional target are mutually exclusive")
		}
		target, comp = TargetSHA, Components{SHA: opts.Commit}
	}
	// `--branch` alone (no positional target) renders a tree URL —
	// Compose handles that case directly when target == TargetNone and
	// opts.Branch is non-empty, so no explicit branch is needed here.

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

	url, err := Compose(target, comp, ComposeOptions{Repo: ref, Tab: tab, Branch: opts.Branch})
	if err != nil {
		return err
	}

	if opts.NoBrowser {
		fmt.Fprintln(opts.IO.Out, url)
		return nil
	}
	fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
	if err := opts.Opener(url); err != nil {
		// Fall back to printing on opener failure (sandbox / missing
		// xdg-open) so the user still gets the link.
		fmt.Fprintln(opts.IO.Out, url)
		fmt.Fprintf(opts.IO.ErrOut, "warning: opener failed: %v\n", err)
	}
	return nil
}

// pickTab enforces the tab-flag mutex and returns the picked tab. At
// most one tab flag may be set.
func pickTab(opts *options) (Tab, error) {
	n := 0
	var picked Tab
	if opts.Projects {
		n++
		picked = TabProjects
	}
	if opts.Releases {
		n++
		picked = TabReleases
	}
	if opts.Settings {
		n++
		picked = TabSettings
	}
	if opts.Wiki {
		n++
		picked = TabWiki
	}
	if n > 1 {
		return "", errors.New("browse: --projects, --releases, --settings, --wiki are mutually exclusive")
	}
	return picked, nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
