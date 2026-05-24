// SPDX-License-Identifier: AGPL-3.0-or-later

// Package ready implements `shithub pr ready`. Flips draft → ready
// (default) or ready → draft when --undo is set.
package ready

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
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/shared/crosskind"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Arg      string
	Repo     string
	Hostname string
	Undo     bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "ready <number-or-url-or-branch>",
		Short: "Mark a pull request as ready for review",
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
	cmd.Flags().BoolVar(&opts.Undo, "undo", false, "convert ready PR back to draft")
	// G14 (F5 / F23): ready→draft is server-side unsupported in v0.1.0
	// (POST /pulls/{n} returns 422 "ready→draft is not supported"). Hide
	// the flag so `pr ready --help` stops advertising broken UX; the flag
	// stays valid on the command line so scripts that pass it get the
	// friendly client-side error below instead of a `--help` regression.
	_ = cmd.Flags().MarkHidden("undo")
	return cmd
}

// Run executes the ready/draft flip.
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

	// H2: cross-namespace verb routing — if the number is actually an
	// issue, surface a friendly redirect rather than "pull request not
	// found". audit-I6: `pr ready` is PR-only, so flag the redirect as
	// asymmetric so the message explains the constraint instead of
	// pointing at `issue ready` (which doesn't exist).
	ic := issues.NewClient(client)
	if err := crosskind.CheckAsymmetric(ctx, ic, pc, ref.Repo.Owner, ref.Repo.Name, ref.Number, "pr", "pr ready", "ready", true); err != nil {
		return err
	}

	draft := opts.Undo
	// G14 (F5 / F23): pre-flight reject ready→draft client-side. The
	// server returns 422 "ready→draft is not supported" with a raw
	// envelope; surfacing the message via the api error path produced
	// `shithub: shithub API: 422 ready→draft is not supported`. Until
	// the server adds support, give the user a single clean line.
	if opts.Undo {
		return fmt.Errorf("pr ready --undo: ready→draft is not yet supported by the server")
	}

	// H5: pre-flight the PR state so we don't print "Marked PR as ready"
	// when the PR is closed/merged or already non-draft. Pre-fix the
	// server silently accepted the no-op PATCH and the CLI lied.
	pr, err := pc.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}
	if pr.Merged {
		return fmt.Errorf("pr ready: PR #%d is already merged", ref.Number)
	}
	if pr.State == "closed" {
		return fmt.Errorf("pr ready: PR #%d is closed; reopen first", ref.Number)
	}
	if !pr.Draft {
		return fmt.Errorf("pr ready: PR #%d is already marked as ready", ref.Number)
	}

	if _, err := pc.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, pulls.EditInput{Draft: &draft}); err != nil {
		return err
	}
	verb := "Marked PR as ready"
	if opts.Undo {
		verb = "Converted PR to draft"
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %s #%d\n", opts.IO.SuccessIcon(), verb, ref.Number)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
