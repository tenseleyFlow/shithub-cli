// SPDX-License-Identifier: AGPL-3.0-or-later

// Package rename implements `shithub repo rename`. Renames the repo on
// the server and (by default) updates the local `origin` remote URL so
// the user's working tree stays usable.
package rename

import (
	"context"
	"fmt"
	"strings"

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

	RepoArg  string
	Repo     string
	Hostname string
	NewName  string
	Yes      bool
	NoRemote bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		Prompter:    f.Prompter,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		GitProtocol: f.GitProtocol,
	}
	cmd := &cobra.Command{
		Use:   "rename [<new-name>]",
		Short: "Rename a repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.NewName = args[0]
			}
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
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&opts.NoRemote, "no-update-remote", false, "don't update the local origin remote URL")
	return cmd
}

// Run executes the rename operation.
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

	if opts.NewName == "" {
		if !opts.IO.IsStdoutTTY() {
			return fmt.Errorf("repo rename: new name required (pass as argument)")
		}
		v, perr := opts.Prompter.Input("New repository name", "")
		if perr != nil {
			return perr
		}
		opts.NewName = strings.TrimSpace(v)
	}
	if opts.NewName == "" {
		return fmt.Errorf("repo rename: new name is empty")
	}
	if opts.NewName == ref.Name {
		return fmt.Errorf("repo rename: new name is the same as the current name")
	}

	if !opts.Yes && opts.IO.IsStdoutTTY() {
		ok, perr := opts.Prompter.Confirm(fmt.Sprintf("Rename %s to %s?", ref.FullName(), opts.NewName), false)
		if perr != nil {
			return perr
		}
		if !ok {
			fmt.Fprintln(opts.IO.ErrOut, "cancelled")
			return nil
		}
	}

	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	rc := repos.NewClient(client)

	renamed, err := rc.Rename(ctx, ref.Owner, ref.Name, opts.NewName)
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Renamed to %s\n", opts.IO.SuccessIcon(), renamed.FullName)

	if opts.NoRemote || opts.GitRunner == nil {
		return nil
	}
	if has, _ := git.RemoteExists(opts.GitRunner, "", "origin"); !has {
		return nil
	}
	protocol := "https"
	if opts.GitProtocol != nil {
		protocol = opts.GitProtocol()
	}
	url := pickURLForRepo(renamed, protocol)
	if err := git.SetRemoteURL(opts.GitRunner, "", "origin", url); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "warning: failed to update origin URL: %v\n", err)
		return nil
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Updated origin -> %s\n", opts.IO.SuccessIcon(), url)
	return nil
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
