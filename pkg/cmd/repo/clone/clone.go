// SPDX-License-Identifier: AGPL-3.0-or-later

// Package clone implements `shithub repo clone`. Resolves the target
// (short name, owner/repo, URL), picks the protocol, shells out to git,
// and — for forks — auto-adds an `upstream` remote pointing at the parent.
package clone

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitProtocol func() string
	GitRunner   git.Runner

	Target      string
	Dir         string
	Extra       []string
	Protocol    string
	Hostname    string
	NoUpstream  bool
	UpstreamRem string
}

// NewCmd builds the cobra command. Any positional args following the
// destination dir are passed verbatim to `git clone` (using `--` is
// supported via cobra's TraverseChildren=false).
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		GitProtocol: f.GitProtocol,
		UpstreamRem: "upstream",
	}
	cmd := &cobra.Command{
		Use:                "clone <repository> [<directory>] [-- <git-clone-flags>...]",
		Short:              "Clone a repository locally",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: false,
		RunE: func(c *cobra.Command, args []string) error {
			opts.Target = args[0]
			// Split positional args from `--`-separated git-clone passthroughs.
			rest := args[1:]
			if sep := c.ArgsLenAtDash(); sep >= 0 {
				positional := rest[:sep-1]
				if len(positional) > 0 {
					opts.Dir = positional[0]
				}
				opts.Extra = rest[sep-1:]
			} else if len(rest) > 0 {
				opts.Dir = rest[0]
				opts.Extra = rest[1:]
			}
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
	cmd.Flags().StringVarP(&opts.Protocol, "protocol", "p", "", "git protocol to use: {https|ssh}")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVar(&opts.NoUpstream, "no-upstream-remote", false, "skip adding an upstream remote when cloning a fork")
	cmd.Flags().StringVar(&opts.UpstreamRem, "upstream-remote-name", "upstream", "name of the upstream remote (when cloning a fork)")
	return cmd
}

// Run executes the clone, returning the resolved local directory on success.
func Run(ctx context.Context, opts *options) error {
	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if host == "" {
		host = config.DefaultHost
	}

	ref, fromURL, err := resolveTarget(opts.Target, host)
	if err != nil {
		return err
	}

	// Fetch repo metadata so we know the canonical case + fork parent.
	// We do this even when the user passed a full URL, so the upstream
	// wiring still works.
	var meta *repos.Repo
	if !fromURL {
		client, herr := opts.HTTPClient(ref.Host)
		if herr != nil {
			return herr
		}
		rc := repos.NewClient(client)
		m, verr := rc.View(ctx, ref.Owner, ref.Name)
		if verr != nil {
			return verr
		}
		meta = m
	}

	protocol := opts.Protocol
	if protocol == "" && opts.GitProtocol != nil {
		protocol = opts.GitProtocol()
	}
	if protocol == "" {
		protocol = "https"
	}

	cloneURL := pickURL(meta, ref, protocol, opts.Target, fromURL)

	resolved, err := git.Clone(opts.GitRunner, cloneURL, opts.Dir, opts.Extra, opts.IO.Out, opts.IO.ErrOut)
	if err != nil {
		return err
	}

	if meta != nil && meta.Fork && meta.Parent != nil && !opts.NoUpstream {
		parent := meta.Parent
		parentURL := pickURLForRepo(parent, protocol)
		if err := git.AddRemote(opts.GitRunner, resolved, opts.UpstreamRem, parentURL); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "warning: failed to add upstream remote: %v\n", err)
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "%s Added %s remote -> %s\n", opts.IO.SuccessIcon(), opts.UpstreamRem, parentURL)
		}
	}

	return nil
}

// resolveTarget accepts the canonical forms:
//
//   - "name" (paired with the authenticated user; rejected here because
//     we don't know the user — surface as error)
//   - "owner/name"
//   - "host/owner/name"
//   - any full URL (https/http/ssh/scp form)
//
// Returns the parsed ref + a bool indicating whether the input was a URL
// (so the caller can skip the metadata fetch when the user gave a direct
// URL — they presumably already know where it points).
func resolveTarget(target, defaultHost string) (shared.RepoRef, bool, error) {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "git@") {
		remote, err := git.ParseRemoteURL(target)
		if err != nil {
			return shared.RepoRef{}, false, fmt.Errorf("repo clone: parse URL: %w", err)
		}
		return shared.RepoRef{Host: remote.Host, Owner: remote.Owner, Name: remote.Repo}, true, nil
	}
	ref, err := shared.ParseRepoArg(target)
	if err != nil {
		return shared.RepoRef{}, false, fmt.Errorf("repo clone: %w", err)
	}
	if ref.Host == "" {
		ref.Host = defaultHost
	}
	return ref, false, nil
}

// pickURL returns the URL to hand to `git clone`. Prefers the server's
// echoed URLs when we have metadata, since they're authoritative for the
// host's canonical case; falls back to the parsed ref / raw input.
func pickURL(meta *repos.Repo, ref shared.RepoRef, protocol, _ string, _ bool) string {
	// H29: pre-fix, a full-URL input was passed through to git as-is.
	// Users who copy `https://shithub.sh/owner/repo` from the web UI
	// got `repository not found` because the server's clone endpoint
	// expects the canonical `https://host/owner/repo.git` form. We
	// have the parsed ref either way — rebuild the URL canonically and
	// drop the raw input.
	if meta != nil {
		return pickURLForRepo(meta, protocol)
	}
	return shared.CloneURL(ref, protocol)
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
