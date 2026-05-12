// SPDX-License-Identifier: AGPL-3.0-or-later

// Package archive implements `shithub repo archive` and `shithub repo unarchive`.
// Both share the same plumbing — they're a thin shell over EditInput.Archived.
package archive

import (
	"context"
	"fmt"

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
	GitRunner   git.Runner

	RepoArg  string
	Repo     string
	Hostname string
	Yes      bool

	// archive true -> flip archived=true (archive cmd); false -> unarchive cmd.
	archive bool
}

// NewArchiveCmd builds `shithub repo archive`.
func NewArchiveCmd(f *cmdutil.Factory) *cobra.Command {
	return newCmd(f, true, "archive", "Archive a repository")
}

// NewUnarchiveCmd builds `shithub repo unarchive`.
func NewUnarchiveCmd(f *cmdutil.Factory) *cobra.Command {
	return newCmd(f, false, "unarchive", "Unarchive a repository")
}

func newCmd(f *cmdutil.Factory, archive bool, use, short string) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		Prompter:    f.Prompter,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		archive:     archive,
	}
	cmd := &cobra.Command{
		Use:   use + " [<owner>/<repo>]",
		Short: short,
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
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "select another repository using the [HOST/]OWNER/REPO format")
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// Run executes the (un)archive operation.
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

	verb := "Archive"
	if !opts.archive {
		verb = "Unarchive"
	}
	if !opts.Yes && opts.IO.IsStdoutTTY() {
		ok, perr := opts.Prompter.Confirm(fmt.Sprintf("%s %s?", verb, ref.FullName()), false)
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

	if opts.archive {
		err = rc.Archive(ctx, ref.Owner, ref.Name)
	} else {
		err = rc.Unarchive(ctx, ref.Owner, ref.Name)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %sd %s\n", opts.IO.SuccessIcon(), verb, ref.FullName())
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
