// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub cache list`.
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

	Key   string
	Ref   string
	Sort  string
	Order string
	Limit int

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Limit:       30,
		Sort:        "last_accessed_at",
		Order:       "desc",
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List Actions cache entries",
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
	cmd.Flags().StringVarP(&opts.Key, "key", "k", "", "filter by exact cache key")
	cmd.Flags().StringVarP(&opts.Ref, "ref", "r", "", "filter by git ref (e.g. refs/heads/trunk)")
	cmd.Flags().StringVarP(&opts.Sort, "sort", "s", "last_accessed_at", "sort field (created_at | last_accessed_at | size_in_bytes)")
	cmd.Flags().StringVarP(&opts.Order, "order", "O", "desc", "sort order (asc | desc)")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "max items to return")
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
	caches, err := ac.ListCaches(ctx, ref.Owner, ref.Name, actions.CacheListOptions{
		Key:   opts.Key,
		Ref:   opts.Ref,
		Sort:  opts.Sort,
		Order: opts.Order,
		Limit: opts.Limit,
	})
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, caches, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, caches)
	return nil
}

func render(io *iostreams.IOStreams, caches []actions.Cache) {
	if len(caches) == 0 {
		fmt.Fprintln(io.ErrOut, "no cache entries found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, c := range caches {
		tp.AddRow(
			fmt.Sprintf("%d", c.ID),
			c.Key,
			c.Ref,
			humanSize(c.SizeInBytes),
			c.LastAccessedAt.Format("2006-01-02"),
		)
	}
	_ = tp.Render()
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
