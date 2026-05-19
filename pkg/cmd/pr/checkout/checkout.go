// SPDX-License-Identifier: AGPL-3.0-or-later

// Package checkout implements `shithub pr checkout`. Fetches the PR head
// into a local tracking branch and checks it out. For cross-fork PRs,
// adds a remote pointing at the fork before fetching.
package checkout

import (
	"context"
	"errors"
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
	GitProtocol func() string
	GitRunner   git.Runner

	Arg      string
	Repo     string
	Hostname string

	BranchName    string
	RecurseSubmod bool
	Force         bool
	Detach        bool
	BaseRemote    string // origin (the repo we checkout into)
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		GitProtocol: f.GitProtocol,
		BaseRemote:  "origin",
	}
	cmd := &cobra.Command{
		Use:   "checkout <number-or-url-or-branch>",
		Short: "Check out a pull request locally",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Arg = args[0]
			if opts.GitRunner == nil {
				r, err := git.FromPath()
				if err != nil {
					return err
				}
				opts.GitRunner = r
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVarP(&opts.BranchName, "branch", "b", "", "override the local branch name")
	cmd.Flags().BoolVarP(&opts.RecurseSubmod, "recurse-submodules", "u", false, "recursively update submodules after checkout")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "force checkout even when the working tree has changes")
	cmd.Flags().BoolVar(&opts.Detach, "detach", false, "check out the PR head in detached HEAD mode")
	return cmd
}

// Run executes the checkout.
//
//nolint:gocyclo // checkout dispatches across same-repo / cross-fork / detached paths.
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, err := resolver.Resolve()
	if err != nil && !strings.Contains(opts.Arg, "://") {
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

	ref, err := prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
	if err != nil {
		return err
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	// Refuse if working tree is dirty unless --force.
	if !opts.Force {
		clean, _ := git.IsClean(opts.GitRunner, "")
		if !clean {
			return errors.New("pr checkout: working tree has uncommitted changes; commit/stash or pass --force")
		}
	}

	pr, err := pc.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}

	if opts.Detach {
		return checkoutDetached(opts, pr)
	}
	return checkoutBranch(opts, pr, ref.Repo)
}

// checkoutDetached fetches the head SHA and checks it out in detached
// HEAD mode. Matches gh's `--detach` flag.
func checkoutDetached(opts *options, pr *pulls.PR) error {
	if pr.Head.SHA == "" {
		return errors.New("pr checkout --detach: PR has no head SHA")
	}
	// Need the SHA locally first. Fetch from origin or the fork remote.
	remote, _, err := pickRemoteForFetch(opts, pr)
	if err != nil {
		return err
	}
	if err := git.Fetch(opts.GitRunner, "", remote, []string{pr.Head.SHA}, opts.IO.Out, opts.IO.ErrOut); err != nil {
		return fmt.Errorf("pr checkout: fetch SHA: %w", err)
	}
	if err := git.CheckoutDetached(opts.GitRunner, "", pr.Head.SHA, opts.IO.Out, opts.IO.ErrOut); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Checked out PR #%d at %s (detached)\n", opts.IO.SuccessIcon(), pr.Number, pr.Head.SHA[:7])
	return nil
}

// checkoutBranch creates/updates a local tracking branch for the PR
// head and checks it out.
func checkoutBranch(opts *options, pr *pulls.PR, baseRepo repocmdshared.RepoRef) error {
	branchName := opts.BranchName
	if branchName == "" {
		branchName = pr.Head.Ref
	}

	// G15 (F6): if we're already on the head branch, git refuses to
	// fetch refs/heads/X:X into the currently-checked-out branch with
	// "refusing to fetch into branch '...' checked out at ...". The
	// correct behavior in that case is a no-op (gh matches this) — the
	// user is already where they want to be.
	if current, _ := git.CurrentBranch(opts.GitRunner, ""); current != "" && current == branchName {
		fmt.Fprintf(opts.IO.ErrOut, "%s Already on PR #%d's head branch (%s); run `git pull` to update\n", opts.IO.SuccessIcon(), pr.Number, branchName)
		_ = baseRepo
		return nil
	}

	remote, isFork, err := pickRemoteForFetch(opts, pr)
	if err != nil {
		return err
	}
	if isFork {
		fmt.Fprintf(opts.IO.ErrOut, "%s Added fork remote %s\n", opts.IO.SuccessIcon(), remote)
	}

	// Fetch the head branch from the chosen remote.
	if err := git.Fetch(opts.GitRunner, "", remote, []string{pr.Head.Ref + ":" + pr.Head.Ref}, opts.IO.Out, opts.IO.ErrOut); err != nil {
		// On second-fetch into an existing local branch, the implicit
		// refspec form can refuse with "would not fast-forward". Retry
		// with the safer "+ref:ref" force update.
		if rerr := git.Fetch(opts.GitRunner, "", remote, []string{"+" + pr.Head.Ref + ":" + pr.Head.Ref}, opts.IO.Out, opts.IO.ErrOut); rerr != nil {
			return fmt.Errorf("pr checkout: fetch head: %w", err)
		}
	}

	exists, _ := git.BranchExists(opts.GitRunner, "", branchName)
	if exists {
		if err := git.CheckoutBranch(opts.GitRunner, "", branchName, opts.Force, opts.IO.Out, opts.IO.ErrOut); err != nil {
			return err
		}
	} else {
		if err := git.CheckoutNewBranch(opts.GitRunner, "", branchName, "", pr.Head.SHA, opts.IO.Out, opts.IO.ErrOut); err != nil {
			return err
		}
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Checked out PR #%d as %s\n", opts.IO.SuccessIcon(), pr.Number, branchName)
	_ = baseRepo
	return nil
}

// pickRemoteForFetch returns the git remote name to fetch from + whether
// it was added on-the-fly for a fork. Same-repo PRs use the local
// `BaseRemote` (typically "origin"). Cross-fork PRs add a remote named
// after the fork owner if not already present.
func pickRemoteForFetch(opts *options, pr *pulls.PR) (string, bool, error) {
	if pr.Head.Repo == nil || pr.Base.Repo == nil {
		return opts.BaseRemote, false, nil
	}
	if pr.Head.Repo.FullName == pr.Base.Repo.FullName {
		return opts.BaseRemote, false, nil
	}
	// Cross-fork: add a remote for the fork.
	forkOwner := ""
	if pr.Head.Repo.Owner != nil {
		forkOwner = pr.Head.Repo.Owner.Login
	}
	if forkOwner == "" {
		return opts.BaseRemote, false, errors.New("pr checkout: fork has no owner; cannot wire remote")
	}
	has, _ := git.RemoteExists(opts.GitRunner, "", forkOwner)
	if !has {
		url := pickURLForRepo(pr.Head.Repo, opts.protocol())
		if err := git.AddRemote(opts.GitRunner, "", forkOwner, url); err != nil {
			return "", false, fmt.Errorf("pr checkout: add fork remote: %w", err)
		}
		return forkOwner, true, nil
	}
	return forkOwner, false, nil
}

func (o *options) protocol() string {
	if o.GitProtocol != nil {
		return o.GitProtocol()
	}
	return "https"
}

func pickURLForRepo(r *pulls.RepoLite, protocol string) string {
	switch strings.ToLower(protocol) {
	case "ssh":
		if r.SSHURL != "" {
			return r.SSHURL
		}
	default:
		if r.CloneURL != "" {
			return r.CloneURL
		}
	}
	// Fall through: construct from FullName.
	return "https://" + "shithub.sh/" + r.FullName + ".git"
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
