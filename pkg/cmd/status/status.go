// SPDX-License-Identifier: AGPL-3.0-or-later

// Package status implements `shithub status`. Renders the personal
// dashboard: assigned issues, assigned PRs, review requests, and
// recent mentions across the orgs the authenticated user belongs to.
//
// v1 uses the search endpoints from C13 as a fan-out (Option B in S50
// §6); a future server-side aggregating endpoint would slot in behind
// the same options struct.
package status

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
)

// PerSectionCap matches gh's `gh status` cap. Users wanting more should
// run the focused list/search command.
const PerSectionCap = 10

// MentionsWindow is the lookback for the mentions section. 30 days
// matches gh.
const MentionsWindow = 30 * 24 * time.Hour

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string

	Hostname  string
	Org       string
	Exclude   []string
	ShowEmpty bool

	// now is overridable in tests so the mentions window stays
	// deterministic.
	now func() time.Time

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		now:         time.Now,
	}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Personal dashboard: assigned issues, PRs, review requests, mentions",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "limit results to a single org")
	cmd.Flags().StringSliceVarP(&opts.Exclude, "exclude", "e", nil, "exclude one or more orgs (repeatable / comma-separated)")
	cmd.Flags().BoolVarP(&opts.ShowEmpty, "show-empty", "s", false, "show empty sections")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Dashboard is the merged payload across all four sections, returned
// to render / export.
type Dashboard struct {
	User           string             `json:"user"`
	AssignedIssues []search.IssueItem `json:"assigned_issues"`
	AssignedPRs    []search.PRItem    `json:"assigned_prs"`
	ReviewRequests []search.PRItem    `json:"review_requests"`
	Mentions       []search.IssueItem `json:"mentions"`
}

// Run executes the dashboard fetch + render.
func Run(ctx context.Context, opts *options) error {
	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	user, err := client.CurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("status: resolve current user: %w", err)
	}
	if user == nil || user.Login == "" {
		return fmt.Errorf("status: current user has no login")
	}

	sc := search.NewClient(client)
	dash, warnings := fanOut(ctx, sc, opts, user.Login)
	dash.User = user.Login

	// Audit A2: when all four sections fan out and fail with the same
	// underlying error (typically "token lacks scope ..." for a too-
	// narrow PAT), the dashboard printed four identical warnings.
	// Collapse duplicates so the user gets one actionable line.
	for _, w := range collapseWarnings(warnings) {
		fmt.Fprintf(opts.IO.ErrOut, "warning: %s\n", w)
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, dash, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, dash, opts.ShowEmpty)
	return nil
}

// fanOut launches four concurrent search calls and returns the merged
// dashboard plus a slice of human-readable warnings for any partial
// failures. We never fail the whole command on a single section's
// error — the user still gets the other three.
func fanOut(ctx context.Context, sc *search.Client, opts *options, login string) (Dashboard, []string) {
	queries := buildQueries(login, opts.Org, opts.Exclude, opts.now().UTC())
	var (
		dash     Dashboard
		warnings []string
		mu       sync.Mutex
		wg       sync.WaitGroup
	)
	addWarning := func(label string, err error) {
		mu.Lock()
		defer mu.Unlock()
		warnings = append(warnings, fmt.Sprintf("%s: %v", label, err))
	}

	wg.Add(4)
	go func() {
		defer wg.Done()
		resp, err := sc.Issues(ctx, queries.assignedIssues, search.Options{Limit: PerSectionCap})
		if err != nil {
			addWarning("assigned issues", err)
			return
		}
		mu.Lock()
		dash.AssignedIssues = capItems(resp.Items, PerSectionCap)
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		resp, err := sc.PullRequests(ctx, queries.assignedPRs, search.Options{Limit: PerSectionCap})
		if err != nil {
			addWarning("assigned PRs", err)
			return
		}
		mu.Lock()
		dash.AssignedPRs = capItems(resp.Items, PerSectionCap)
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		resp, err := sc.PullRequests(ctx, queries.reviewRequests, search.Options{Limit: PerSectionCap})
		if err != nil {
			addWarning("review requests", err)
			return
		}
		mu.Lock()
		dash.ReviewRequests = capItems(resp.Items, PerSectionCap)
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		resp, err := sc.Issues(ctx, queries.mentions, search.Options{Limit: PerSectionCap})
		if err != nil {
			addWarning("mentions", err)
			return
		}
		mu.Lock()
		dash.Mentions = capItems(resp.Items, PerSectionCap)
		mu.Unlock()
	}()
	wg.Wait()
	return dash, warnings
}

// collapseWarnings deduplicates warnings whose error text (the part
// after "section_label: ") is identical. When N sections all fail
// the same way (the missing-scope case is the canonical example),
// the output collapses to a single "every section: <error>" line.
// Returns the input untouched when every warning is distinct.
func collapseWarnings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	const sep = ": "
	groups := make(map[string][]string)
	order := []string{}
	for _, w := range in {
		idx := strings.Index(w, sep)
		if idx < 0 {
			// Unparseable; keep as-is.
			groups[w] = append(groups[w], "")
			order = appendIfNew(order, w)
			continue
		}
		label, body := w[:idx], w[idx+len(sep):]
		if _, ok := groups[body]; !ok {
			order = append(order, body)
		}
		groups[body] = append(groups[body], label)
	}
	out := make([]string, 0, len(order))
	for _, body := range order {
		labels := groups[body]
		if len(labels) <= 1 {
			label := ""
			if len(labels) == 1 {
				label = labels[0]
			}
			if label == "" {
				out = append(out, body)
			} else {
				out = append(out, label+": "+body)
			}
			continue
		}
		// All sections share this error — say so plainly.
		out = append(out, "every section: "+body)
	}
	return out
}

func appendIfNew(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// capItems clips a slice to the per-section cap. Search.Client already
// honors Limit, but we double-check to keep the contract local — a
// future server-side aggregating endpoint may not respect Limit per
// section.
func capItems[T any](xs []T, n int) []T {
	if len(xs) <= n {
		return xs
	}
	return xs[:n]
}
