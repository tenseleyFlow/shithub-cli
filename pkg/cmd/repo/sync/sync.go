// SPDX-License-Identifier: AGPL-3.0-or-later

// Package sync implements `shithub repo sync`. Sync a fork with upstream
// via the server's merge-upstream endpoint; on its absence or refusal,
// fall back to local `git fetch upstream && git merge --ff-only`.
package sync

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	RepoArg  string
	Repo     string
	Source   string
	Branch   string
	Hostname string
	Force    bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "sync [<destination-repo>]",
		Short: "Sync a repository fork with its upstream parent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
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
	cmd.Flags().StringVar(&opts.Source, "source", "", "source repository in `<owner>/<repo>` form (default: fork's parent)")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "branch to sync (default: default branch)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "force-update destination branch (destructive)")
	return cmd
}

// Run executes the sync operation. Prefers server-side merge-upstream;
// on 4xx/5xx (or missing fork relationship) falls back to a local
// `git fetch && merge --ff-only` against the upstream remote.
func Run(ctx context.Context, opts *options) error {
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

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	rc := repos.NewClient(client)

	branch := opts.Branch
	if branch == "" {
		// Server side: default branch is implied when branch field is empty.
		// We still need *something* for the API contract, so peek at metadata.
		meta, verr := rc.View(ctx, ref.Owner, ref.Name)
		if verr != nil {
			return verr
		}
		branch = meta.DefaultBranch
	}

	result, err := rc.MergeUpstream(ctx, ref.Owner, ref.Name, repos.MergeUpstreamInput{Branch: branch})
	if err == nil {
		fmt.Fprintf(opts.IO.ErrOut, "%s %s\n", opts.IO.SuccessIcon(), nonEmpty(result.Message, "synced from upstream"))
		return nil
	}

	// Server-side path failed; try local fallback when we're inside a working tree.
	if opts.GitRunner == nil {
		return err
	}
	isRepo, _ := git.IsRepo(opts.GitRunner, "")
	if !isRepo {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "server merge-upstream unavailable (%v); falling back to local fetch+ff-only\n", err)

	if ferr := git.Fetch(opts.GitRunner, "", "upstream", []string{branch}, opts.IO.ErrOut, opts.IO.ErrOut); ferr != nil {
		return ferr
	}
	if merr := git.MergeFastForward(opts.GitRunner, "", "upstream/"+branch, opts.IO.ErrOut, opts.IO.ErrOut); merr != nil {
		return merr
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s synced %s from upstream/%s locally\n", opts.IO.SuccessIcon(), ref.FullName(), branch)
	return nil
}

func nonEmpty(a, fallback string) string {
	if a == "" {
		return fallback
	}
	return a
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
