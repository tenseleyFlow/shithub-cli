// SPDX-License-Identifier: AGPL-3.0-or-later

// Package delete implements `shithub variable delete <name>`.
package delete

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner
	Prompter    prompter.Prompter

	Name     string
	Repo     string
	Hostname string
	Org      string
	Yes      bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Prompter:    f.Prompter,
	}
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a variable",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
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
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "delete an org-level variable")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// Run executes.
func Run(ctx context.Context, opts *options) error {
	if !opts.Yes && opts.IO.IsStdinTTY() && opts.Prompter != nil {
		confirmed, err := opts.Prompter.Confirm(
			fmt.Sprintf("Delete variable %q?", opts.Name), false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(opts.IO.ErrOut, "cancelled")
			return nil
		}
	}

	if opts.Org != "" {
		client, err := opts.HTTPClient(hostOrDefault(opts.DefaultHost))
		if err != nil {
			return err
		}
		if err := secrets.NewClient(client).DeleteOrgVariable(ctx, opts.Org, opts.Name); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted variable %s from org %s\n",
			opts.IO.SuccessIcon(), opts.Name, opts.Org)
		return nil
	}

	ref, err := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}.Resolve()
	if err != nil {
		return err
	}
	client, err := opts.HTTPClient(ref.Host)
	if err != nil {
		return err
	}
	if err := secrets.NewClient(client).DeleteRepoVariable(ctx, ref.Owner, ref.Name, opts.Name); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted variable %s from %s/%s\n",
		opts.IO.SuccessIcon(), opts.Name, ref.Owner, ref.Name)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
