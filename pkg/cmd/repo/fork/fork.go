// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fork implements `shithub repo fork`. Forks the target to the
// authenticated user (or `--org`); if --clone is set (or the user
// confirms interactively), clones the fork and rotates the local
// remotes — origin → upstream, new origin → fork.
package fork

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	Prompter    prompter.Prompter
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitProtocol func() string
	GitRunner   git.Runner

	RepoArg           string
	Repo              string
	Hostname          string
	Org               string
	Name              string
	Clone             bool
	cloneSet          bool
	Remote            bool
	remoteSet         bool
	RemoteName        string
	DefaultBranchOnly bool

	// PollInterval throttles the wait loop after creating a fork until
	// the server reports the new repo is queryable. Production NewCmd
	// sets 500ms; the zero value (used by tests that construct options
	// directly) skips the readiness probe entirely so they don't have to
	// stub the View endpoint.
	PollInterval time.Duration
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:           f.IOStreams,
		Prompter:     f.Prompter,
		HTTPClient:   f.HTTPClient,
		DefaultHost:  f.DefaultHost,
		GitProtocol:  f.GitProtocol,
		RemoteName:   "origin",
		PollInterval: 500 * time.Millisecond,
	}
	cmd := &cobra.Command{
		Use:   "fork [<owner>/<repo>]",
		Short: "Create a fork of a repository",
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
			opts.cloneSet = c.Flags().Changed("clone")
			opts.remoteSet = c.Flags().Changed("remote")
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVar(&opts.Org, "org", "", "create the fork in an organization")
	cmd.Flags().StringVar(&opts.Name, "fork-name", "", "rename the forked repository")
	cmd.Flags().BoolVar(&opts.Clone, "clone", false, "clone the fork after creation")
	cmd.Flags().BoolVar(&opts.Remote, "remote", false, "add a remote for the fork in the current git working tree")
	cmd.Flags().StringVar(&opts.RemoteName, "remote-name", "origin", "name of the fork remote")
	cmd.Flags().BoolVar(&opts.DefaultBranchOnly, "default-branch-only", false, "only include the default branch in the fork")
	return cmd
}

// Run executes the fork operation.
//
//nolint:gocyclo // fork stitches together resolve+fork+clone+remote-rotation.
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

	// B4: detect the own-repo case up front so the user gets a clear
	// message instead of the server's generic 422. We only consult the
	// authenticated user when the caller hasn't passed --org (an org
	// destination is, by definition, not "yourself").
	if opts.Org == "" {
		me, merr := client.CurrentUser(ctx)
		if merr == nil && strings.EqualFold(me.Login, ref.Owner) {
			return fmt.Errorf("repo fork: cannot fork your own repository (%s/%s); use --org to fork into an organization", ref.Owner, ref.Name)
		}
	}

	in := repos.ForkInput{
		Organization:      opts.Org,
		Name:              opts.Name,
		DefaultBranchOnly: opts.DefaultBranchOnly,
	}
	fork, err := rc.Fork(ctx, ref.Owner, ref.Name, in)
	if err != nil {
		return err
	}
	// E-audit E17: the server's POST /forks response sometimes omits
	// full_name (or returns a partial envelope). Compute a best-effort
	// fallback from the inputs so the success line never reads
	// "Created fork " with a trailing space.
	forkName := strings.TrimSpace(fork.FullName)
	if forkName == "" {
		forkOwner := opts.Org
		if forkOwner == "" {
			if me, merr := client.CurrentUser(ctx); merr == nil {
				forkOwner = me.Login
			}
		}
		forkRepoName := opts.Name
		if forkRepoName == "" {
			forkRepoName = ref.Name
		}
		if forkOwner != "" && forkRepoName != "" {
			forkName = forkOwner + "/" + forkRepoName
		}
	}
	upstream := ref.Owner + "/" + ref.Name
	if forkName != "" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Created fork %s from %s\n", opts.IO.SuccessIcon(), forkName, upstream)
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "%s Created fork from %s\n", opts.IO.SuccessIcon(), upstream)
	}

	// shithub's Fork endpoint returns immediately, but the server may
	// still be provisioning the new repo's git storage. Poll View()
	// until it returns success so a subsequent clone doesn't race the
	// background job. We bound the wait so a stuck job doesn't wedge
	// the command — timeout falls through with a warning and lets git
	// surface its own error if the storage truly isn't ready.
	if err := waitForForkReady(ctx, opts, rc, fork); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "warning: %v\n", err)
	}

	// gh prompts interactively when neither --clone nor --no-clone is given.
	// We mirror that: TTY + flag not set + nothing to disambiguate the intent.
	clone := opts.Clone
	if !opts.cloneSet && opts.IO.IsStdoutTTY() && opts.GitRunner != nil {
		ans, perr := opts.Prompter.Confirm("Clone the fork?", false)
		if perr != nil {
			if !errors.As(perr, new(prompter.NotInteractive)) {
				return perr
			}
		} else {
			clone = ans
		}
	}

	// If we're inside a git repo whose remote points at the upstream and
	// neither --remote nor --no-remote was passed, gh asks the user.
	inWorkingTree := opts.GitRunner != nil && isInsideUpstreamWorkingTree(opts.GitRunner, ref)
	addRemote := opts.Remote
	if !opts.remoteSet && inWorkingTree && opts.IO.IsStdoutTTY() && !clone {
		ans, perr := opts.Prompter.Confirm("Add a remote for the fork?", false)
		if perr == nil {
			addRemote = ans
		}
	}

	protocol := "https"
	if opts.GitProtocol != nil {
		protocol = opts.GitProtocol()
	}

	switch {
	case clone:
		return cloneFork(opts, fork, protocol)
	case addRemote:
		return rotateRemoteInPlace(opts, fork, protocol)
	}
	return nil
}

// cloneFork clones the fork into a new dir and adds upstream pointing at
// the parent. The fork URL is preferred from the server response; the
// parent URL is composed because we may not have its echoed clone URL.
func cloneFork(opts *options, fork *repos.Repo, protocol string) error {
	if opts.GitRunner == nil {
		return fmt.Errorf("repo fork: git binary required for --clone")
	}
	url := pickURLForRepo(fork, protocol)
	dst, err := git.Clone(opts.GitRunner, url, "", nil, opts.IO.Out, opts.IO.ErrOut)
	if err != nil {
		return err
	}
	if fork.Parent != nil {
		parentURL := pickURLForRepo(fork.Parent, protocol)
		if err := git.AddRemote(opts.GitRunner, dst, "upstream", parentURL); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "warning: failed to add upstream remote: %v\n", err)
		}
	}
	return nil
}

// rotateRemoteInPlace handles the "I'm already in a clone of upstream;
// add the fork as origin and rotate the original origin -> upstream"
// flow. Idempotent: it'll detect existing `upstream` and skip the rename.
func rotateRemoteInPlace(opts *options, fork *repos.Repo, protocol string) error {
	dir := "" // cwd
	url := pickURLForRepo(fork, protocol)
	// If `upstream` doesn't exist, rename origin -> upstream first.
	hasUpstream, _ := git.RemoteExists(opts.GitRunner, dir, "upstream")
	hasOrigin, _ := git.RemoteExists(opts.GitRunner, dir, "origin")
	if !hasUpstream && hasOrigin {
		if err := git.RenameRemote(opts.GitRunner, dir, "origin", "upstream"); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Renamed origin -> upstream\n", opts.IO.SuccessIcon())
	}
	// Now add the fork as origin (or whatever opts.RemoteName is).
	name := opts.RemoteName
	if name == "" {
		name = "origin"
	}
	has, _ := git.RemoteExists(opts.GitRunner, dir, name)
	switch {
	case has:
		if err := git.SetRemoteURL(opts.GitRunner, dir, name, url); err != nil {
			return err
		}
	default:
		if err := git.AddRemote(opts.GitRunner, dir, name, url); err != nil {
			return err
		}
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Set %s -> %s\n", opts.IO.SuccessIcon(), name, url)
	return nil
}

// isInsideUpstreamWorkingTree reports whether the current dir's origin
// remote points at ref (i.e., the user is forking from a clone of the
// upstream). Used to gate the interactive remote-rotation prompt.
func isInsideUpstreamWorkingTree(r git.Runner, ref shared.RepoRef) bool {
	rem, err := git.ResolveRemote("", "origin")
	if err != nil {
		return false
	}
	return strings.EqualFold(rem.Owner, ref.Owner) && strings.EqualFold(rem.Repo, ref.Name)
}

func pickURLForRepo(r *repos.Repo, protocol string) string {
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
	owner, name := splitFullName(r.FullName)
	return shared.CloneURL(shared.RepoRef{Owner: owner, Name: name}, protocol)
}

func splitFullName(s string) (string, string) {
	if i := strings.IndexByte(s, '/'); i > 0 {
		return s[:i], s[i+1:]
	}
	return "", s
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}

// maxForkWait caps the readiness probe so a stuck server doesn't wedge
// the command. 5s is enough headroom for shithub's typical fork-init
// path (a few hundred ms of background work) without dominating wall
// time on the happy path where the first probe succeeds.
const maxForkWait = 5 * time.Second

// waitForForkReady polls View(fork.Owner.Login, fork.Name) until it
// succeeds, which is the proxy signal that the server has materialized
// the new repo's storage and a subsequent `git clone` won't race the
// background provisioning job. A nil return means ready; a non-nil
// return means the timeout fired and the caller should warn-but-proceed.
func waitForForkReady(ctx context.Context, opts *options, rc *repos.Client, fork *repos.Repo) error {
	if rc == nil || fork == nil || opts.PollInterval <= 0 {
		return nil
	}
	deadline := time.Now().Add(maxForkWait)
	for {
		if _, err := rc.View(ctx, fork.Owner.Login, fork.Name); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fork not ready after %s; continuing anyway", maxForkWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.PollInterval):
		}
	}
}
