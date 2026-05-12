// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub gpg-key list`.
package list

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
	"github.com/tenseleyFlow/shithub-cli/internal/output"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
)

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
		Use:   "list",
		Short: "List your GPG public keys",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	output.AddFlags(cmd, &opts.Exporter)
	return cmd
}

// Run executes the listing.
func Run(ctx context.Context, opts *options) error {
	host := config.DefaultHost
	if opts.Hostname != "" {
		h, err := config.ValidateHost(opts.Hostname)
		if err != nil {
			return err
		}
		host = h
	}
	client, err := opts.HTTPClient(host)
	if err != nil {
		return err
	}
	kc := keys.NewClient(client)
	list, err := kc.ListGPG(ctx)
	if err != nil {
		return err
	}

	if opts.Exporter.Active() {
		return output.Export(opts.IO.Out, opts.Exporter, exporter{}, list, opts.IO.IsStdoutTTY())
	}
	render(opts.IO, list)
	return nil
}

func render(io *iostreams.IOStreams, list []keys.GPGKey) {
	if len(list) == 0 {
		fmt.Fprintln(io.ErrOut, "no GPG keys registered")
		return
	}
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, k := range list {
		tp.AddRow(
			fmt.Sprintf("%d", k.ID),
			k.KeyID,
			joinEmails(k.Emails),
			capabilities(k),
			k.CreatedAt.Format("2006-01-02"),
		)
	}
	_ = tp.Render()
}

// joinEmails flattens the verified-emails slice into a comma list for
// the table. Unverified entries get a trailing "?" so the user can spot
// them.
func joinEmails(es []keys.GPGEmail) string {
	parts := make([]string, 0, len(es))
	for _, e := range es {
		s := e.Email
		if !e.Verified {
			s += "?"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ",")
}

// capabilities renders a small flag string ala `gpg --list-keys`:
//   - c — certify
//   - s — sign
//   - e — encrypt (comms or storage)
//   - a — authenticate
func capabilities(k keys.GPGKey) string {
	var b strings.Builder
	if k.CanCertify {
		b.WriteByte('c')
	}
	if k.CanSign {
		b.WriteByte('s')
	}
	if k.CanEncryptComms || k.CanEncryptStorage {
		b.WriteByte('e')
	}
	if k.CanAuthenticate {
		b.WriteByte('a')
	}
	return b.String()
}
