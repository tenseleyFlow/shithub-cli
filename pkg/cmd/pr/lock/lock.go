// SPDX-License-Identifier: AGPL-3.0-or-later

// Package lock implements `shithub pr lock` and `shithub pr unlock`.
// PRs share the lock endpoint with issues (shithub mirrors GitHub's
// model), so we route through the issues package directly.
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
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

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

// NewLockCmd builds `shithub pr lock`.
func NewLockCmd(f *cmdutil.Factory) *cobra.Command {
	return newCmd(f, true, "lock", "Lock a pull request")
}

// NewUnlockCmd builds `shithub pr unlock`.
func NewUnlockCmd(f *cmdutil.Factory) *cobra.Command {
	return newCmd(f, false, "unlock", "Unlock a pull request")
}

func newCmd(f *cmdutil.Factory, lock bool, use, short string) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		lock:        lock,
	}
	cmd := &cobra.Command{
		Use:   use + " <number-or-url-or-branch>",
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

// Run executes the (un)lock.
func Run(ctx context.Context, opts *options) error {
	if opts.lock && opts.Reason != "" {
		if _, ok := validReasons[opts.Reason]; !ok {
			return fmt.Errorf("pr lock: invalid --reason %q", opts.Reason)
		}
	}

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
	ic := issues.NewClient(client)

	ref, err := prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
	if err != nil {
		return err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	if opts.lock {
		if err := ic.Lock(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, opts.Reason); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Locked PR #%d\n", opts.IO.SuccessIcon(), ref.Number)
		return nil
	}
	if err := ic.Unlock(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Unlocked PR #%d\n", opts.IO.SuccessIcon(), ref.Number)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
