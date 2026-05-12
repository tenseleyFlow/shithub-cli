// SPDX-License-Identifier: AGPL-3.0-or-later

// Package del implements `shithub gpg-key delete`.
package del

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
	"github.com/tenseleyFlow/shithub-cli/internal/prompter"
)

type options struct {
	IO          *iostreams.IOStreams
	Prompter    prompter.Prompter
	HTTPClient  func(host string) (*api.Client, error)
	DefaultHost func() string

	ID       int64
	Yes      bool
	Hostname string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{
		IO:          f.IOStreams,
		Prompter:    f.Prompter,
		HTTPClient:  f.HTTPClient,
		DefaultHost: f.DefaultHost,
	}
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a GPG key from your shithub account",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil || id <= 0 {
				return fmt.Errorf("gpg-key delete: %q is not a valid id", args[0])
			}
			opts.ID = id
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "skip the confirmation prompt")
	return cmd
}

// Run deletes the key.
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

	if !opts.Yes {
		if opts.IO.NeverPrompt() {
			return errors.New("gpg-key delete: pass --yes to confirm in non-interactive mode")
		}
		ok, perr := opts.Prompter.Confirm(fmt.Sprintf("Delete GPG key id=%d?", opts.ID), false)
		if perr != nil {
			return perr
		}
		if !ok {
			fmt.Fprintln(opts.IO.ErrOut, "cancelled")
			return nil
		}
	}
	if err := kc.DeleteGPG(ctx, opts.ID); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted GPG key id=%d\n", opts.IO.SuccessIcon(), opts.ID)
	return nil
}
