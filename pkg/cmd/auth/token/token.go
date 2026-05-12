// SPDX-License-Identifier: AGPL-3.0-or-later

// Package token implements `shithub auth token`. Prints the stored token
// for a host to stdout (no trailing newline). Used as a building block by
// git credential helpers and by shell pipelines that need the raw bearer.
package token

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// Options drives Run.
type Options struct {
	IO       *iostreams.IOStreams
	Hosts    func() (config.Hosts, error)
	Keyring  func() config.KeyringStore
	Hostname string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:      f.IOStreams,
		Hosts:   f.Hosts,
		Keyring: f.Keyring,
	}
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the authenticated token for a host",
		Long: `Print the stored token for a host (no trailing newline).

Useful when shell-scripting against the shithub REST API or when
configuring a git credential helper manually:

    git config credential.https://shithub.sh.helper '!shithub auth git-credential'
`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host whose token to print")
	return cmd
}

// Run resolves the host and writes the token to stdout. Returns an error
// when no token is configured.
func Run(ctx context.Context, opts *Options) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	host := config.ResolveHost(opts.Hostname, hosts)
	defaultHost := hosts.DefaultHostName()

	token, _, err := config.ResolveToken(opts.Keyring(), hosts, defaultHost, host)
	if err != nil {
		if errors.Is(err, config.ErrNoToken) {
			return fmt.Errorf("auth: not authenticated to %s; run `shithub auth login --hostname %s`", host, host)
		}
		return err
	}
	if _, err := opts.IO.Out.Write([]byte(token)); err != nil {
		return err
	}
	_ = ctx
	return nil
}
