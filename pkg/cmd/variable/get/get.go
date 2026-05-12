// SPDX-License-Identifier: AGPL-3.0-or-later

// Package get implements `shithub variable get <name>`. Prints the
// variable's plaintext value to stdout.
package get

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Name     string
	Repo     string
	Hostname string
	Org      string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Print a variable's value",
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
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "fetch an org-level variable")
	return cmd
}

// Run executes.
func Run(ctx context.Context, opts *options) error {
	var (
		v   *secrets.Variable
		err error
	)
	if opts.Org != "" {
		client, herr := opts.HTTPClient(hostOrDefault(opts.DefaultHost))
		if herr != nil {
			return herr
		}
		v, err = secrets.NewClient(client).GetOrgVariable(ctx, opts.Org, opts.Name)
	} else {
		ref, rerr := repocmdshared.Resolver{
			RepoFlag:    opts.Repo,
			Hostname:    opts.Hostname,
			DefaultHost: hostOrDefault(opts.DefaultHost),
			GitRunner:   opts.GitRunner,
		}.Resolve()
		if rerr != nil {
			return rerr
		}
		client, cerr := opts.HTTPClient(ref.Host)
		if cerr != nil {
			return cerr
		}
		v, err = secrets.NewClient(client).GetRepoVariable(ctx, ref.Owner, ref.Name, opts.Name)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.IO.Out, v.Value)
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
