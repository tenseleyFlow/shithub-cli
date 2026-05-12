// SPDX-License-Identifier: AGPL-3.0-or-later

// Package lock implements `shithub issue lock` and `shithub issue unlock`.
// Both share their plumbing; an internal bool flips behavior.
package lock

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

// validReasons matches gh's lock_reason enum. Empty reason is also valid.
var validReasons = map[string]struct{}{
	"off_topic":  {},
	"too_heated": {},
	"resolved":   {},
	"spam":       {},
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

	lock bool
}

// NewLockCmd builds `shithub issue lock`.
func NewLockCmd(f *cmdutil.Factory) *cobra.Command { return newCmd(f, true, "lock", "Lock an issue") }

// NewUnlockCmd builds `shithub issue unlock`.
func NewUnlockCmd(f *cmdutil.Factory) *cobra.Command {
	return newCmd(f, false, "unlock", "Unlock an issue")
}

func newCmd(f *cmdutil.Factory, lock bool, use, short string) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		lock:        lock,
	}
	cmd := &cobra.Command{
		Use:   use + " <number-or-url>",
		Short: short,
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
	if lock {
		cmd.Flags().StringVarP(&opts.Reason, "reason", "r", "", "lock reason: {off_topic|too_heated|resolved|spam}")
	}
	return cmd
}

// Run executes the (un)lock operation.
func Run(ctx context.Context, opts *options) error {
	if opts.lock && opts.Reason != "" {
		if _, ok := validReasons[opts.Reason]; !ok {
			return fmt.Errorf("issue lock: invalid --reason %q (want off_topic/too_heated/resolved/spam)", opts.Reason)
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

	if opts.lock {
		if err := ic.Lock(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, opts.Reason); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Locked #%d\n", opts.IO.SuccessIcon(), ref.Number)
		return nil
	}
	if err := ic.Unlock(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Unlocked #%d\n", opts.IO.SuccessIcon(), ref.Number)
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
