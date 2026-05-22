// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub run list`.
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

	Workflow string
	Branch   string
	Event    string
	Actor    string
	Status   string
	Limit    int

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Limit:       30,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List workflow runs",
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
	cmd.Flags().StringVarP(&opts.Workflow, "workflow", "w", "", "filter by workflow name or id (.yml file accepted)")
	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "filter by branch")
	cmd.Flags().StringVarP(&opts.Event, "event", "e", "", "filter by event (push, pull_request, workflow_dispatch, ...)")
	cmd.Flags().StringVarP(&opts.Actor, "user", "u", "", "filter by triggering user login")
	cmd.Flags().StringVarP(&opts.Status, "status", "s", "", "filter by status (queued|in_progress|completed|success|failure|...)")
	cmdutil.AddLimitFlag(cmd, &opts.Limit, 30, "max items to return")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the listing.
func Run(ctx context.Context, opts *options) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}
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
	runs, err := ac.ListRuns(ctx, ref.Owner, ref.Name, actions.RunListOptions{
		WorkflowFile: opts.Workflow,
		Branch:       opts.Branch,
		Event:        opts.Event,
		Actor:        opts.Actor,
		Status:       opts.Status,
		Limit:        opts.Limit,
	})
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, runs, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, runs)
	return nil
}

func render(io *iostreams.IOStreams, runs []actions.WorkflowRun) {
	if len(runs) == 0 {
		fmt.Fprintln(io.ErrOut, "no workflow runs found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, r := range runs {
		conclusion := r.Conclusion
		if conclusion == "" {
			conclusion = r.Status
		}
		tp.AddRow(
			fmt.Sprintf("%d", r.ID),
			r.Name,
			conclusion,
			r.HeadBranch,
			r.Event,
			r.UpdatedAt.Format("2006-01-02"),
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
