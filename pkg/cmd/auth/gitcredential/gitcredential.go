// SPDX-License-Identifier: AGPL-3.0-or-later

// Package gitcredential implements the hidden `shithub auth git-credential`
// command — the credential-helper protocol target wired up by `auth
// setup-git`. git invokes us with `get` (and ignores `store` / `erase`
// without complaint, matching gh's behavior).
package gitcredential

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/auth"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

// Options drives Run.
type Options struct {
	IO      *iostreams.IOStreams
	Hosts   func() (config.Hosts, error)
	Keyring func() config.KeyringStore
}

// NewCmd builds the cobra command. Hidden because users never type it
// directly — git invokes it via the credential-helper config that
// `auth setup-git` writes.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:      f.IOStreams,
		Hosts:   f.Hosts,
		Keyring: f.Keyring,
	}
	cmd := &cobra.Command{
		Use: "git-credential",
		// G14 (F24): pre-fix Hidden:true left this subcommand invisible
		// in `auth --help`, but the `auth token --help` text explicitly
		// references it for manual git-credential helper configuration.
		// Unhide so the discoverability matches the doc; the "Internal:"
		// prefix in Short signals the role.
		Short: "Internal: git credential helper protocol target",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return Run(c.Context(), opts, args[0])
		},
	}
	return cmd
}

// Run dispatches the get/store/erase verb. Only `get` does anything;
// store/erase exit 0 silently so git's protocol expectations are met.
func Run(_ context.Context, opts *Options, verb string) error {
	switch verb {
	case "store", "erase":
		// We don't manage credentials via git; ignore politely.
		return nil
	case "get":
		return runGet(opts)
	default:
		return fmt.Errorf("git-credential: unknown verb %q", verb)
	}
}

// runGet reads a credential request from stdin, looks up the matching
// host's token, and writes the response. Missing tokens yield silent
// no-output so git falls through to its next credential helper.
func runGet(opts *Options) error {
	req, err := auth.ReadCredentialRequest(opts.IO.In)
	if err != nil {
		return err
	}
	if req.Host == "" {
		return errors.New("git-credential: empty host in request")
	}

	host := config.NormalizeHost(req.Host)
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	entry, ok := hosts[host]
	if !ok {
		// Silent miss: git keeps walking its helper chain.
		return nil
	}
	token, _, err := config.ResolveToken(opts.Keyring(), hosts, hosts.DefaultHostName(), host)
	if err != nil {
		return nil
	}
	return auth.WriteCredentialResponse(opts.IO.Out, req.Protocol, entry.User, token)
}
