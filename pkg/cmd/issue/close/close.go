// SPDX-License-Identifier: AGPL-3.0-or-later

// Package close implements `shithub issue close`. Patches the issue to
// state=closed; --reason maps onto the state_reason enum, --comment posts
// a closing comment before the state flip (matches gh's ordering so the
// timeline reads correctly).
package close

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
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	issueshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/shared/crosskind"
)

// validReasons matches GitHub's state_reason enum. Empty reason is also
// valid (server picks "completed" as default).
var validReasons = map[string]struct{}{
	"completed":   {},
	"not_planned": {},
	"duplicate":   {},
}

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Arg      string
	Repo     string
	Hostname string
	Reason   string
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
		Use:   "close <number-or-url>",
		Short: "Close an issue",
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
	cmd.Flags().StringVarP(&opts.Reason, "reason", "r", "", "close reason: {completed|not_planned|duplicate}")
	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "comment to post before closing")
	return cmd
}

// Run executes the close operation.
func Run(ctx context.Context, opts *options) error {
	if opts.Reason != "" {
		if _, ok := validReasons[opts.Reason]; !ok {
			return fmt.Errorf("issue close: invalid --reason %q (want completed/not_planned/duplicate)", opts.Reason)
		}
	}

	ref, err := resolve(opts)
	if err != nil {
		return err
	}

	client, err := opts.HTTPClient(ref.Repo.Host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	// H2: cross-namespace verb routing. If the number is a PR (not an
	// issue), surface a friendly redirect instead of letting the server
	// PATCH return a raw 422 about the shared issue+PR table.
	if err := crosskind.Check(ctx, ic, pc, ref.Repo.Owner, ref.Repo.Name, ref.Number, "issue", "issue close", "close"); err != nil {
		return err
	}

	if opts.Comment != "" {
		if _, cerr := ic.AddComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, opts.Comment); cerr != nil {
			return cerr
		}
	}

	state := "closed"
	in := issues.EditInput{State: &state}
	if opts.Reason != "" {
		reason := opts.Reason
		in.StateReason = &reason
	}
	if _, err := ic.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, in); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Closed #%d\n", opts.IO.SuccessIcon(), ref.Number)
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
