// SPDX-License-Identifier: AGPL-3.0-or-later

// Package logout implements `shithub auth logout`. Removes a host's
// keyring entry + hosts.yml record. Refuses to operate without an
// explicit --hostname when multiple hosts are configured (no surprise
// deletions).
package logout

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
)

// Options drives Run.
type Options struct {
	IO       *iostreams.IOStreams
	Prompter prompter.Prompter
	Hosts    func() (config.Hosts, error)
	Keyring  func() config.KeyringStore

	Hostname string
	Yes      bool
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{
		IO:       f.IOStreams,
		Prompter: f.Prompter,
		Hosts:    f.Hosts,
		Keyring:  f.Keyring,
	}
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove a stored credential for a shithub host",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host to log out of")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// Run performs the logout. Returns a friendly error if no hosts are
// configured or if --hostname is ambiguous.
func Run(_ context.Context, opts *Options) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return errors.New("auth: not logged in to any host")
	}

	host := config.NormalizeHost(opts.Hostname)
	if host == "" {
		if len(hosts) > 1 {
			return errors.New("auth: multiple hosts configured; pass --hostname <host>")
		}
		// Single host: pick it.
		for h := range hosts {
			host = h
		}
	}
	entry, ok := hosts[host]
	if !ok {
		return fmt.Errorf("auth: not authenticated to %s", host)
	}

	if !opts.Yes {
		ok, err := confirmLogout(opts, host, entry.User)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(opts.IO.ErrOut, "Logout cancelled.")
			return nil
		}
	}

	// Best-effort keyring delete; absent secrets are not an error.
	if entry.User != "" {
		_ = config.DeleteToken(opts.Keyring(), host, entry.User)
	}
	hosts.Delete(host)
	if err := hosts.Save(); err != nil {
		return fmt.Errorf("auth: persist logout: %w", err)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Logged out of %s\n", opts.IO.SuccessIcon(), host)
	return nil
}

// confirmLogout prompts the user for confirmation. Falls back to a
// silent "yes" when stdin is non-interactive (rather than a hang). The
// caller already had the opportunity to pass --yes; reaching this branch
// without --yes in a non-interactive context is a configuration mistake
// we surface clearly.
func confirmLogout(opts *Options, host, user string) (bool, error) {
	if opts.IO.NeverPrompt() {
		return false, errors.New("auth: non-interactive logout requires --yes")
	}
	msg := fmt.Sprintf("Log out of %s as %s?", host, user)
	return opts.Prompter.Confirm(msg, false)
}
