// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub org list`. Lists the orgs the
// authenticated user belongs to; an optional positional <user> selects
// the public org list of another user instead.
package list

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/orgs"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
)

// DefaultLimit matches gh's `org list` default.
const DefaultLimit = 30

// MaxLimit caps `--limit` so a typo doesn't kick off a thousand-page
// walk.
const MaxLimit = 1000

type options struct {
	IO          *iostreams.IOStreams
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string

	User     string // positional: empty = authenticated user
	Hostname string
	Limit    int

	Exporter output.Options
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
		Limit:       DefaultLimit,
	}
	cmd := &cobra.Command{
		Use:   "list [<user>]",
		Short: "List organizations you belong to",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.User = args[0]
			}
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", DefaultLimit, "maximum number of organizations to list")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the listing.
func Run(ctx context.Context, opts *options) error {
	if err := cmdutil.ValidateLimit(opts.Limit); err != nil {
		return err
	}
	if opts.Limit > MaxLimit {
		opts.Limit = MaxLimit
	}
	host := opts.Hostname
	if host == "" && opts.DefaultHost != nil {
		host = opts.DefaultHost()
	}
	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	oc := orgs.NewClient(client)

	listOpts := orgs.ListOptions{Limit: opts.Limit}
	var all []orgs.Org
	if opts.User == "" {
		all, err = oc.ListAuthenticated(ctx, listOpts)
	} else {
		all, err = oc.ListUser(ctx, opts.User, listOpts)
	}
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, all, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, all)
	return nil
}

// render writes a table to stdout. Empty result hits stderr with a
// hint and exits clean.
func render(io *iostreams.IOStreams, list []orgs.Org) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no organizations found")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, o := range list {
		tp.AddRow(o.Login, roleString(o), countsString(o), badges(o))
	}
	_ = tp.Render()
}

// roleString surfaces the user's role on the org — server-supplied on
// /user/orgs. Empty when the listing endpoint doesn't echo it.
func roleString(o orgs.Org) string {
	if o.Role == "" {
		return ""
	}
	return o.Role
}

// countsString renders "<repos> repos, <members> members" when both
// counts are available. Skips fields that come back zero so we don't
// invent data the server didn't send.
func countsString(o orgs.Org) string {
	parts := []string{}
	if o.PublicRepos > 0 {
		parts = append(parts, fmt.Sprintf("%d repos", o.PublicRepos))
	}
	if o.MembersCount > 0 {
		parts = append(parts, fmt.Sprintf("%d members", o.MembersCount))
	}
	return strings.Join(parts, ", ")
}

// badges renders the marker column for suspended / verified orgs.
func badges(o orgs.Org) string {
	parts := []string{}
	if o.Suspended {
		parts = append(parts, "suspended")
	}
	if o.IsVerified {
		parts = append(parts, "verified")
	}
	return strings.Join(parts, ",")
}
