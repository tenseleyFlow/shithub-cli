// SPDX-License-Identifier: AGPL-3.0-or-later

// Package checks implements `shithub pr checks`. Lists check runs for
// the PR's head SHA, optionally polling until conclusive (--watch),
// optionally filtered to required checks. --fail-fast (with --watch)
// exits non-zero on the first failure rather than waiting for all to
// finish.
package checks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	checksclient "github.com/tenseleyFlow/shithub-cli/internal/checks"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// DefaultInterval matches gh's `gh pr checks --watch` interval.
const DefaultInterval = 5 * time.Second

// DefaultTimeout caps the watch loop so a stuck CI doesn't wedge the
// terminal. Override via --timeout.
const DefaultTimeout = 30 * time.Minute

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	GitRunner   git.Runner

	Arg      string
	Repo     string
	Hostname string

	Watch    bool
	Required bool
	FailFast bool
	Interval time.Duration
	Timeout  time.Duration

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Interval:    DefaultInterval,
		Timeout:     DefaultTimeout,
	}
	cmd := &cobra.Command{
		Use:   "checks [<number-or-url-or-branch>]",
		Short: "Show check runs for a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
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
	cmd.Flags().BoolVarP(&opts.Watch, "watch", "w", false, "poll until all checks are conclusive")
	cmd.Flags().BoolVar(&opts.Required, "required", false, "show only required checks")
	cmd.Flags().BoolVar(&opts.FailFast, "fail-fast", false, "with --watch, exit on first failure")
	cmd.Flags().DurationVar(&opts.Interval, "interval", DefaultInterval, "polling interval for --watch")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", DefaultTimeout, "max wait for --watch")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the checks query.
//
// same head-SHA resolution.
//
//nolint:gocyclo // watch loop + render + exporter dispatch share the
func Run(ctx context.Context, opts *options) error {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	fb, rerr := resolver.Resolve()
	if rerr != nil && (opts.Arg == "" || !strings.Contains(opts.Arg, "://")) {
		return rerr
	}
	if rerr != nil {
		fb = repocmdshared.RepoRef{}
	}
	client, err := opts.HTTPClient(fb.Host)
	if err != nil {
		return err
	}
	pc := pulls.NewClient(client)

	var ref prshared.PRRef
	if opts.Arg == "" {
		branch := prshared.CurrentBranchFromGit(opts.GitRunner, "")
		if branch == "" {
			return errors.New("pr checks: pass a number/URL/branch (couldn't detect current branch)")
		}
		pr, err := prshared.FindPRByBranch(ctx, pc, fb, branch)
		if err != nil {
			return err
		}
		ref = prshared.PRRef{Repo: fb, Number: pr.Number}
	} else {
		ref, err = prshared.ParsePRArg(ctx, pc, opts.Arg, fb)
		if err != nil {
			return err
		}
	}
	if ref.Repo.Host == "" {
		ref.Repo.Host = fb.Host
	}

	pr, err := pc.View(ctx, ref.Repo.Owner, ref.Repo.Name, ref.Number)
	if err != nil {
		return err
	}
	if pr.Head.SHA == "" {
		return errors.New("pr checks: PR has no head SHA")
	}

	cc := checksclient.NewClient(client)

	if opts.Watch {
		return watchLoop(ctx, opts, cc, ref, pr.Head.SHA)
	}

	runs, err := cc.ListForRef(ctx, ref.Repo.Owner, ref.Repo.Name, pr.Head.SHA)
	if err != nil {
		return err
	}
	runs = filter(runs, opts.Required)
	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, runs, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, runs)
	return summaryExit(runs)
}

// watchLoop polls until every run is conclusive (or --fail-fast triggers).
// Each iteration replaces the previous render so the terminal shows the
// live matrix instead of a scrolling tail.
func watchLoop(ctx context.Context, opts *options, cc *checksclient.Client, ref prshared.PRRef, sha string) error {
	deadline := time.Now().Add(opts.Timeout)
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	for {
		runs, err := cc.ListForRef(ctx, ref.Repo.Owner, ref.Repo.Name, sha)
		if err != nil {
			return err
		}
		runs = filter(runs, opts.Required)
		render(opts.IO, runs)

		done := allCompleted(runs)
		if opts.FailFast && hasFailure(runs) {
			return fmt.Errorf("pr checks: failing check detected (--fail-fast)")
		}
		if done {
			return summaryExit(runs)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("pr checks: --watch timed out after %s", opts.Timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// filter narrows the list to required checks only when --required is set.
func filter(runs []checksclient.CheckRun, requiredOnly bool) []checksclient.CheckRun {
	if !requiredOnly {
		return runs
	}
	out := make([]checksclient.CheckRun, 0, len(runs))
	for _, r := range runs {
		if r.Required {
			out = append(out, r)
		}
	}
	return out
}

// render writes a status table to stdout.
func render(io *iostreams.IOStreams, runs []checksclient.CheckRun) {
	if len(runs) == 0 {
		fmt.Fprintln(io.ErrOut, "no checks reported")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, r := range runs {
		tp.AddRow(statusIcon(r), r.Name, conclusion(r), elapsed(r))
	}
	_ = tp.Render()
}

// allCompleted reports whether every run reached the completed status.
func allCompleted(runs []checksclient.CheckRun) bool {
	for _, r := range runs {
		if !r.IsCompleted() {
			return false
		}
	}
	return true
}

// hasFailure reports whether any completed run failed.
func hasFailure(runs []checksclient.CheckRun) bool {
	for _, r := range runs {
		if r.IsFailure() {
			return true
		}
	}
	return false
}

// summaryExit returns a non-nil error when any completed run failed —
// so `pr checks` exits non-zero, scriptable in CI.
func summaryExit(runs []checksclient.CheckRun) error {
	if hasFailure(runs) {
		return errors.New("pr checks: one or more checks failed")
	}
	return nil
}

// statusIcon picks a small status glyph for each check.
func statusIcon(r checksclient.CheckRun) string {
	switch {
	case !r.IsCompleted():
		return "◯"
	case r.IsSuccess():
		return "✓"
	default:
		return "✗"
	}
}

// conclusion renders a human-friendly status string. Pending runs say
// "queued" or "in_progress"; completed runs surface the conclusion.
func conclusion(r checksclient.CheckRun) string {
	if !r.IsCompleted() {
		return r.Status
	}
	return r.Conclusion
}

// elapsed renders the run duration when both timestamps are present.
func elapsed(r checksclient.CheckRun) string {
	if r.StartedAt == nil {
		return ""
	}
	end := time.Now()
	if r.CompletedAt != nil {
		end = *r.CompletedAt
	}
	return end.Sub(*r.StartedAt).Round(time.Second).String()
}

// exporter projects each run for --json.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"completedAt", "conclusion", "link", "name",
		"required", "startedAt", "status",
	}
}

func (exporter) Filter(v any) (any, error) {
	runs, ok := v.([]checksclient.CheckRun)
	if !ok {
		return nil, fmt.Errorf("pr checks exporter: want []checks.CheckRun, got %T", v)
	}
	out := make([]map[string]any, 0, len(runs))
	for _, r := range runs {
		out = append(out, map[string]any{
			"completedAt": r.CompletedAt,
			"conclusion":  r.Conclusion,
			"link":        r.HTMLURL,
			"name":        r.Name,
			"required":    r.Required,
			"startedAt":   r.StartedAt,
			"status":      r.Status,
		})
	}
	return out, nil
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
