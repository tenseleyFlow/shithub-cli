// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub secret list` over the repo and org
// scopes. The org scope requires --org; otherwise we resolve a repo.
package list

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
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
	Org      string
	App      string

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		App:         "actions",
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List secrets for a repository or organization",
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
	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "list org-level secrets")
	cmd.Flags().StringVar(&opts.App, "app", "actions", "scope: actions (only supported app today)")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run dispatches by scope.
func Run(ctx context.Context, opts *options) error {
	if opts.App != "" && opts.App != "actions" {
		return fmt.Errorf("secret list: only --app actions is supported (got %q)", opts.App)
	}

	var entries []secrets.Secret
	switch {
	case opts.Org != "":
		client, err := opts.HTTPClient(hostOrDefault(opts.DefaultHost))
		if err != nil {
			return err
		}
		entries, err = secrets.NewClient(client).ListOrgSecrets(ctx, opts.Org)
		if err != nil {
			return err
		}
	default:
		ref, err := resolveRepo(opts)
		if err != nil {
			return err
		}
		client, err := opts.HTTPClient(ref.Host)
		if err != nil {
			return err
		}
		entries, err = secrets.NewClient(client).ListRepoSecrets(ctx, ref.Owner, ref.Name)
		if err != nil {
			return err
		}
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, entries, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, entries, opts.Org != "")
	return nil
}

func resolveRepo(opts *options) (repocmdshared.RepoRef, error) {
	resolver := repocmdshared.Resolver{
		RepoFlag:    opts.Repo,
		Hostname:    opts.Hostname,
		DefaultHost: hostOrDefault(opts.DefaultHost),
		GitRunner:   opts.GitRunner,
	}
	return resolver.Resolve()
}

func render(io *iostreams.IOStreams, entries []secrets.Secret, orgScope bool) {
	if len(entries) == 0 {
		fmt.Fprintln(io.ErrOut, "no secrets found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, s := range entries {
		row := []string{s.Name, s.UpdatedAt.Format("2006-01-02")}
		if orgScope {
			row = append(row, s.Visibility)
		}
		tp.AddRow(row...)
	}
	_ = tp.Render()
}

func hostOrDefault(fn func() string) string {
	if fn == nil {
		return ""
	}
	return fn()
}
