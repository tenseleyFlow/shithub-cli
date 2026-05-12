// SPDX-License-Identifier: AGPL-3.0-or-later

// Package watch implements `shithub run watch <run-id>` — a polling
// loop that prints status transitions until the run reaches a terminal
// state.  shithub S41a does not currently offer an event stream;
// when one lands we'll plug it in behind the same flag surface.
package watch

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/api"
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

	RunID      string
	Repo       string
	Hostname   string
	Interval   time.Duration
	ExitStatus bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Interval:    3 * time.Second,
	}
	cmd := &cobra.Command{
		Use:   "watch <run-id>",
		Short: "Watch a workflow run until it completes",
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
	cmd.Flags().DurationVarP(&opts.Interval, "interval", "i", 3*time.Second, "polling interval (e.g. 2s, 500ms)")
	cmd.Flags().BoolVar(&opts.ExitStatus, "exit-status", false, "exit non-zero if the run failed")
	return cmd
}

// Run polls.
func Run(ctx context.Context, opts *options) error {
	id, err := strconv.ParseInt(opts.RunID, 10, 64)
	if err != nil {
		return fmt.Errorf("run watch: %q is not a numeric run id", opts.RunID)
	}
	if opts.Interval <= 0 {
		opts.Interval = 3 * time.Second
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

	var lastStatus string
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		r, gerr := ac.GetRun(ctx, ref.Owner, ref.Name, id)
		if gerr != nil {
			return gerr
		}
		if r.Status != lastStatus {
			fmt.Fprintf(opts.IO.ErrOut, "%s status: %s\n", r.UpdatedAt.Format("15:04:05"), r.Status)
			lastStatus = r.Status
		}
		if r.IsCompleted() {
			conclusion := r.Conclusion
			if conclusion == "" {
				conclusion = "(no conclusion)"
			}
			fmt.Fprintf(opts.IO.Out, "%s Run %d completed with conclusion: %s\n",
				icon(opts.IO, r.IsFailure()), r.ID, conclusion)
			if opts.ExitStatus && r.IsFailure() {
				return errors.New("run failed")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func icon(io *iostreams.IOStreams, failure bool) string {
	if failure {
		return io.FailureIcon()
	}
	return io.SuccessIcon()
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
