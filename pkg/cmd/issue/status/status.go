// SPDX-License-Identifier: AGPL-3.0-or-later

// Package status implements `shithub issue status`. A cross-repo dashboard
// of "issues relevant to you": assigned, mentioned, authored. Each
// category is a separate /issues query; a shared limit caps each list
// independently so a flood of mentions doesn't drown out assignments.
package status

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
)

// SectionLimit caps each section (assigned/mentioned/authored). Total
// output is bounded at 3 × SectionLimit.
const SectionLimit = 10

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string

	Hostname string

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
		Use:   "status",
		Short: "Show issues relevant to you across all repositories",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the dashboard fetch.
func Run(ctx context.Context, opts *options) error {
	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	ic := issues.NewClient(client)

	assigned, err := ic.ListAcrossRepos(ctx, "assigned", issues.ListOptions{State: "open", Limit: SectionLimit})
	if err != nil {
		return fmt.Errorf("issue status: assigned: %w", err)
	}
	mentioned, err := ic.ListAcrossRepos(ctx, "mentioned", issues.ListOptions{State: "open", Limit: SectionLimit})
	if err != nil {
		return fmt.Errorf("issue status: mentioned: %w", err)
	}
	authored, err := ic.ListAcrossRepos(ctx, "created", issues.ListOptions{State: "open", Limit: SectionLimit})
	if err != nil {
		return fmt.Errorf("issue status: authored: %w", err)
	}

	if opts.Exporter.Active() {
		bundle := map[string][]issues.Issue{
			"assigned":  assigned,
			"mentioned": mentioned,
			"authored":  authored,
		}
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, bundle, opts.IO.IsStdoutTTY())
	}

	renderSection(opts.IO, "Issues assigned to you", assigned)
	renderSection(opts.IO, "Issues mentioning you", mentioned)
	renderSection(opts.IO, "Issues opened by you", authored)
	return nil
}

// renderSection emits a small subheader plus a table of (repo, #n, title).
// Empty sections collapse to a single dim "nothing here" line so the
// dashboard stays scannable.
func renderSection(io *iostreams.IOStreams, title string, list []issues.Issue) {
	fmt.Fprintf(io.Out, "%s\n", title)
	if len(list) == 0 {
		fmt.Fprintln(io.Out, "  nothing here")
		fmt.Fprintln(io.Out)
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, i := range list {
		repo := ""
		if i.Repository != nil {
			repo = i.Repository.FullName
		}
		tp.AddRow(
			"  "+repo,
			fmt.Sprintf("#%d", i.Number),
			truncate(i.Title, 70),
		)
	}
	_ = tp.Render()
	fmt.Fprintln(io.Out)
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// exporter is the JSON projection for the dashboard. Returns the three
// sections as a stable-keyed object.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{"assigned", "authored", "mentioned"}
}

func (exporter) Filter(v any) (any, error) {
	b, ok := v.(map[string][]issues.Issue)
	if !ok {
		return nil, fmt.Errorf("issue status exporter: want map[string][]issues.Issue, got %T", v)
	}
	out := map[string][]map[string]any{
		"assigned":  projectAll(b["assigned"]),
		"mentioned": projectAll(b["mentioned"]),
		"authored":  projectAll(b["authored"]),
	}
	return out, nil
}

func projectAll(list []issues.Issue) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, i := range list {
		repo := ""
		if i.Repository != nil {
			repo = i.Repository.FullName
		}
		out = append(out, map[string]any{
			"number":     i.Number,
			"title":      i.Title,
			"repository": repo,
			"url":        i.HTMLURL,
			"state":      i.State,
			"updatedAt":  i.UpdatedAt,
		})
	}
	return out
}
