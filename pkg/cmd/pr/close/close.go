// SPDX-License-Identifier: AGPL-3.0-or-later

// Package close implements `shithub pr close`. Patches state=closed,
// optionally posts a closing comment, optionally deletes the head
// branch locally + on the remote.
package close

import (
	"context"
	"errors"
	"fmt"
	"io"
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

	Arg          string
	Repo         string
	Hostname     string
	Comment      string
	DeleteBranch bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "close <number-or-url-or-branch>",
		Short: "Close a pull request",
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
	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "comment to post before closing")
	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "delete the head branch locally and on the remote")
	return cmd
}

// Run executes the close.
func Run(ctx context.Context, opts *options) error {
	ref, pr, err := resolveAndView(ctx, opts)
	if err != nil {
		return err
	}

	client, err := opts.HTTPClient(ref.Repo.Host)
	if err != nil {
		return err
	}
	pc := pulls.NewClient(client)
	ic := issues.NewClient(client)

	if opts.Comment != "" {
		if _, err := ic.AddComment(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, opts.Comment); err != nil {
			return err
		}
	}

	state := "closed"
	if _, err := pc.Edit(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number, pulls.EditInput{State: &state}); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Closed PR #%d\n", opts.IO.SuccessIcon(), ref.Number)

	if opts.DeleteBranch {
		if err := deleteHeadBranch(opts, ref, pr); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "warning: --delete-branch: %v\n", err)
		}
	}
	return nil
}

// deleteHeadBranch removes the PR's head branch both locally (when the
// caller has it checked out anywhere) and on the remote. Best-effort —
// errors are surfaced as warnings, not fatal.
func deleteHeadBranch(opts *options, ref prshared.PRRef, pr *pulls.PR) error {
	if opts.GitRunner == nil {
		return errors.New("git binary not available")
	}
	head := pr.Head.Ref
	if head == "" {
		return errors.New("PR has no head ref")
	}
	// Local: only delete when a branch by that name exists.
	if exists, _ := git.BranchExists(opts.GitRunner, "", head); exists {
		// Don't delete the current branch — switch to base first.
		if cur, _ := git.CurrentBranch(opts.GitRunner, ""); cur == head {
			if err := git.CheckoutBranch(opts.GitRunner, "", pr.Base.Ref, false, io.Discard, io.Discard); err != nil {
				return fmt.Errorf("switch to base: %w", err)
			}
		}
		if err := opts.GitRunner.Run("", []string{"branch", "-D", head}, io.Discard, opts.IO.ErrOut); err != nil {
			return fmt.Errorf("local delete: %w", err)
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted local branch %s\n", opts.IO.SuccessIcon(), head)
	}
	// Remote delete via `git push origin --delete <branch>`. Only attempt
	// when the PR head is on the same repo as base (else we'd push to a
	// remote we don't own).
	if pr.Head.Repo != nil && pr.Base.Repo != nil && pr.Head.Repo.FullName == pr.Base.Repo.FullName {
		if err := opts.GitRunner.Run("", []string{"push", "origin", "--delete", head}, io.Discard, opts.IO.ErrOut); err != nil {
			return fmt.Errorf("remote delete: %w", err)
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted remote branch %s\n", opts.IO.SuccessIcon(), head)
	}
	_ = ref
	return nil
}

func resolveAndView(ctx context.Context, opts *options) (prshared.PRRef, *pulls.PR, error) {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := resolver.Resolve()
	if rerr != nil && !strings.Contains(opts.Arg, "://") {
		return prshared.PRRef{}, nil, rerr
	}
	if rerr != nil {
		fb = repocmdshared.RepoRef{}
	}
	client, err := opts.HTTPClient(fb.Host)
	if err != nil {
		return prshared.PRRef{}, nil, err
	}
	pc := pulls.NewClient(client)
	ref, err := prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
	if err != nil {
		return prshared.PRRef{}, nil, err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}
	pr, err := pc.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		// H2: if pc.View returned not-found, check whether the number
		// is actually an issue and surface a friendly redirect instead
		// of "pull request not found".
		if api.IsNotFoundError(err) {
			ic := issues.NewClient(client)
			if cerr := crosskind.Check(ctx, ic, pc, ref.Repo.Owner, ref.Repo.Name, ref.Number, "pr", "pr close", "close"); cerr != nil {
				return prshared.PRRef{}, nil, cerr
			}
		}
		return prshared.PRRef{}, nil, err
	}
	return ref, pr, nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
