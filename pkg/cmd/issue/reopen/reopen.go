// SPDX-License-Identifier: AGPL-3.0-or-later

// Package reopen implements `shithub issue reopen`. Mirror of `issue
// close` with the opposite state target and an optional --comment.
package reopen

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
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
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
	Comment  string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "reopen <number-or-url>",
		Short: "Reopen a closed issue",
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
	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "comment to post before reopening")
	return cmd
}

// Run executes the reopen operation.
func Run(ctx context.Context, opts *options) error {
	ref, err := resolve(opts)
	if err != nil {
		return err
	}

	client, err := opts.HTTPClient(ref.Repo.Host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)

	if opts.Comment != "" {
		if _, cerr := ic.AddComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, opts.Comment); cerr != nil {
			return cerr
		}
	}

	state := "open"
	if _, err := ic.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, issues.EditInput{State: &state}); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Reopened #%d\n", opts.IO.SuccessIcon(), ref.Number)
	return nil
}

func resolve(opts *options) (issueshared.IssueRef, error) {
	rs := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := rs.Resolve()
	if rerr != nil && !strings.Contains(opts.Arg, "://") {
		return issueshared.IssueRef{}, rerr
	}
	if rerr != nil {
		fb = repocmdshared.RepoRef{}
	}
	ref, _, err := issueshared.ParseIssueArg(opts.Arg, fb)
	if err != nil {
		return issueshared.IssueRef{}, err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}
	return ref, nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
