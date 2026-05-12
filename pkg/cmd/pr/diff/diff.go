// SPDX-License-Identifier: AGPL-3.0-or-later

// Package diff implements `shithub pr diff`. Renders the raw unified
// diff for a PR (via shithub's .diff endpoint) or, with --name-only,
// the list of changed files (via /pulls/{n}/files).
package diff

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Arg      string
	Repo     string
	Hostname string

	NameOnly bool
	Patch    bool
	Color    string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Color:       "auto",
	}
	cmd := &cobra.Command{
		Use:   "diff [<number-or-url-or-branch>]",
		Short: "Show the diff of a pull request",
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
	cmd.Flags().BoolVar(&opts.NameOnly, "name-only", false, "show only the changed file names")
	cmd.Flags().BoolVar(&opts.Patch, "patch", false, "show the patch format (alias for default diff)")
	cmd.Flags().StringVar(&opts.Color, "color", "auto", "color output: {auto|always|never}")
	return cmd
}

// Run executes the diff operation.
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
	if opts.Arg == "" {
		branch := prshared.CurrentBranchFromGit(opts.GitRunner, "")
		if branch == "" {
			return fmt.Errorf("pr diff: pass a number/URL/branch (couldn't detect current branch)")
		}
		pr, err := prshared.FindPRByBranch(ctx, pc, fb, branch)
		if err != nil {
			return err
		}
		ref = prshared.PRRef{Repo: fb, Number: pr.Number}
	} else {
		ref, err = prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
		if err != nil {
			return err
		}
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	if opts.NameOnly {
		files, err := pc.ListFiles(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
		if err != nil {
			return err
		}
		for _, f := range files {
			fmt.Fprintln(opts.IO.Out, f.Filename)
		}
		return nil
	}

	raw, err := pc.Diff(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}
	_, _ = opts.IO.Out.Write(raw)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
