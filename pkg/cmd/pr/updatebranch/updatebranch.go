// SPDX-License-Identifier: AGPL-3.0-or-later

// Package updatebranch implements `shithub pr update-branch`. Calls the
// server's merge-with-base endpoint (rebase variant via header). On
// 409 / non-fast-forward, surfaces the server's message verbatim.
package updatebranch

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
	Rebase   bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "update-branch <number-or-url-or-branch>",
		Short: "Update a pull request branch with the latest base",
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
	cmd.Flags().BoolVar(&opts.Rebase, "rebase", false, "use rebase strategy instead of merge")
	return cmd
}

// Run executes the update-branch operation.
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := resolver.Resolve()
	if rerr != nil && !strings.Contains(opts.Arg, "://") {
		return rerr
	}
	if rerr != nil {
		fb = repocmdshared.RepoRef{}
	}
	client, err := opts.HTTPClient(fb.Host)
	if err != nil {
		return err
	}
	pc := pulls.NewClient(client)

	ref, err := prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
	if err != nil {
		return err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	result, err := pc.UpdateBranch(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, pulls.UpdateBranchInput{}, opts.Rebase)
	if err != nil {
		return err
	}
	msg := result.Message
	if msg == "" {
		msg = "PR branch updated with base"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %s\n", opts.IO.SuccessIcon(), msg)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
