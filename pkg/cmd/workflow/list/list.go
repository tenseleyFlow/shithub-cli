// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub workflow list`. Renders the
// `/actions/workflows` listing — by default only active workflows;
// `--all` includes disabled ones (matches gh).
package list

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Repo     string
	Hostname string
	All      bool
	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflows for a repository",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
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
	cmd.Flags().BoolVarP(&opts.All, "all", "a", false, "include disabled workflows")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the listing.
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
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
	ac := actions.NewClient(client)
	list, err := ac.ListWorkflows(ctx, ref.Owner, ref.Name)
	if err != nil {
		return err
	}
	if !opts.All {
		filtered := list[:0]
		for _, w := range list {
			if w.IsActive() {
				filtered = append(filtered, w)
			}
		}
		list = filtered
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, list, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, list)
	return nil
}

func render(io *iostreams.IOStreams, list []actions.Workflow) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no workflows found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, w := range list {
		tp.AddRow(
			fmt.Sprintf("%d", w.ID),
			w.Name,
			w.Path,
			w.State,
		)
	}
	_ = tp.Render()
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
