// SPDX-License-Identifier: AGPL-3.0-or-later

// Package view implements `shithub run view <run-id>`.
package view

import (
	"context"
	"errors"
	"fmt"
	"strconv"

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

	RunID      string
	Repo       string
	Hostname   string
	Web        bool
	ExitStatus bool
	ShowJobs   bool
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
		Use:   "view <run-id>",
		Short: "Show details for a workflow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.RunID = args[0]
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
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the run in a browser")
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "exit non-zero if the run failed")
	cmd.Flags().BoolVar(&opts.ShowJobs, "jobs", true, "show per-job status in the rendered view")
	return cmd
}

// Run executes the lookup.
func Run(ctx context.Context, opts *options) error {
	id, err := strconv.ParseInt(opts.RunID, 10, 64)
	if err != nil {
		return fmt.Errorf("run view: %q is not a numeric run id", opts.RunID)
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
	r, err := ac.GetRun(ctx, ref.Owner, ref.Name, id)
	if err != nil {
		return err
	}
	if opts.Web {
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", r.HTMLURL)
		return opts.Opener(r.HTMLURL)
	}

	renderHeader(opts.IO, r)

	if opts.ShowJobs {
		jobs, jerr := ac.ListJobs(ctx, ref.Owner, ref.Name, id)
		if jerr != nil {
			fmt.Fprintf(opts.IO.ErrOut, "run view: jobs lookup failed: %v\n", jerr)
		} else if len(jobs) > 0 {
			renderJobs(opts.IO, jobs)
		}
	}

	if opts.ExitStatus && r.IsFailure() {
		return errors.New("run failed")
	}
	return nil
}

func renderHeader(io *iostreams.IOStreams, r *actions.WorkflowRun) {
	conclusion := r.Conclusion
	if conclusion == "" {
		conclusion = r.Status
	}
	fmt.Fprintf(io.Out, "%s · run #%d (%d)\n", r.Name, r.RunNumber, r.ID)
	fmt.Fprintf(io.Out, "  status:     %s\n", conclusion)
	fmt.Fprintf(io.Out, "  event:      %s\n", r.Event)
	fmt.Fprintf(io.Out, "  branch:     %s\n", r.HeadBranch)
	if r.HeadSHA != "" {
		fmt.Fprintf(io.Out, "  sha:        %s\n", shortSHA(r.HeadSHA))
	}
	if r.TriggeringActor != nil && r.TriggeringActor.Login != "" {
		fmt.Fprintf(io.Out, "  actor:      %s\n", r.TriggeringActor.Login)
	} else if r.Actor != nil && r.Actor.Login != "" {
		fmt.Fprintf(io.Out, "  actor:      %s\n", r.Actor.Login)
	}
	fmt.Fprintf(io.Out, "  created:    %s\n", r.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(io.Out, "  updated:    %s\n", r.UpdatedAt.Format("2006-01-02 15:04:05"))
	if r.HTMLURL != "" {
		fmt.Fprintf(io.Out, "  url:        %s\n", r.HTMLURL)
	}
}

func renderJobs(io *iostreams.IOStreams, jobs []actions.Job) {
	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Jobs:")
	for _, j := range jobs {
		conclusion := j.Conclusion
		if conclusion == "" {
			conclusion = j.Status
		}
		fmt.Fprintf(io.Out, "  %-12s  %s (#%d)\n", conclusion, j.Name, j.ID)
		for _, s := range j.Steps {
			sc := s.Conclusion
			if sc == "" {
				sc = s.Status
			}
			fmt.Fprintf(io.Out, "      %-10s  %s\n", sc, s.Name)
		}
	}
}

func shortSHA(s string) string {
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
