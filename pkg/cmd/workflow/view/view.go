// SPDX-License-Identifier: AGPL-3.0-or-later

// Package view implements `shithub workflow view <id-or-file>`.
package view

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/browser"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner
	Opener      func(url string) error

	Selector string
	Repo     string
	Hostname string
	Web      bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Opener:      browser.Open,
	}
	cmd := &cobra.Command{
		Use:   "view <id-or-file>",
		Short: "Show details for a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Selector = args[0]
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
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the workflow in a browser")
	return cmd
}

// Run executes the lookup.
func Run(ctx context.Context, opts *options) error {
	if opts.Selector == "" {
		return errors.New("workflow view: missing selector")
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
	wf, err := ac.GetWorkflow(ctx, ref.Owner, ref.Name, opts.Selector)
	if err != nil {
		return err
	}
	if opts.Web {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", wf.HTMLURL)
		return opts.Opener(wf.HTMLURL)
	}

	fmt.Fprintf(opts.IO.Out, "%s\n", wf.Name)
	fmt.Fprintf(opts.IO.Out, "  id:    %d\n", wf.ID)
	fmt.Fprintf(opts.IO.Out, "  file:  %s\n", wf.Path)
	fmt.Fprintf(opts.IO.Out, "  state: %s\n", wf.State)
	if wf.HTMLURL != "" {
		fmt.Fprintf(opts.IO.Out, "  url:   %s\n", wf.HTMLURL)
	}

	// Recent runs (top 5) for context. We deliberately keep this short;
	// users wanting more drop to `shithub run list --workflow <file>`.
	runs, err := ac.ListRuns(ctx, ref.Owner, ref.Name, actions.RunListOptions{
		WorkflowFile: opts.Selector, Limit: 5,
	})
	if err != nil {
		return nil //nolint:nilerr // run lookup is best-effort
	}
	if len(runs) > 0 {
		fmt.Fprintln(opts.IO.Out)
		fmt.Fprintln(opts.IO.Out, "Recent runs:")
		for _, r := range runs {
			conclusion := r.Conclusion
			if conclusion == "" {
				conclusion = r.Status
			}
			fmt.Fprintf(opts.IO.Out, "  #%d  %-12s  %s  %s\n",
				r.RunNumber, conclusion, r.HeadBranch, r.UpdatedAt.Format("2006-01-02"))
		}
	}
	return nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
