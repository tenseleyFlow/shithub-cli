// SPDX-License-Identifier: AGPL-3.0-or-later

// Package view implements `shithub org view <org>`. Renders the org
// profile envelope (description, location, repo / member counts).
// `--web` opens shithub's org page.
package view

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/browser"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/orgs"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
)

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string
	Opener      func(url string) error

	Org      string
	Hostname string
	Web      bool

	Exporter output.Options
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
		Use:   "view <org>",
		Short: "Show an organization's profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Org = args[0]
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open the org page in a browser")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the view.
func Run(ctx context.Context, opts *options) error {
	if opts.Org == "" {
		return errors.New("org view: missing org name")
	}
	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	if host == "" {
		host = config.DefaultHost
	}

	if opts.Web {
		url := fmt.Sprintf("https://%s/%s", host, opts.Org)
		fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", url)
		return opts.Opener(url)
	}

	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	oc := orgs.NewClient(client)
	org, err := oc.Get(ctx, opts.Org)
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, *org, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, org)
	return nil
}

// render writes a profile summary to stdout. Empty fields are skipped.
func render(io *iostreams.IOStreams, o *orgs.Org) {
	if o.Name != "" {
		fmt.Fprintf(io.Out, "%s (@%s)\n", o.Name, o.Login)
	} else {
		fmt.Fprintf(io.Out, "@%s\n", o.Login)
	}
	if o.Description != "" {
		fmt.Fprintln(io.Out, o.Description)
	}
	fmt.Fprintln(io.Out)

	type row struct {
		label, value string
	}
	rows := []row{
		{"Location", o.Location},
		{"Email", o.Email},
		{"Website", o.Blog},
		{"Twitter", o.TwitterUser},
		{"Public repos", intOrEmpty(o.PublicRepos)},
		{"Members", intOrEmpty(o.MembersCount)},
		{"Created", timeOrEmpty(o)},
	}
	for _, r := range rows {
		if r.value == "" {
			continue
		}
		fmt.Fprintf(io.Out, "%-13s %s\n", r.label+":", r.value)
	}
	if o.Suspended {
		fmt.Fprintln(io.ErrOut, "warning: this org is suspended")
	}
}

func intOrEmpty(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

func timeOrEmpty(o *orgs.Org) string {
	if o.CreatedAt.IsZero() {
		return ""
	}
	return o.CreatedAt.Format("2006-01-02")
}
