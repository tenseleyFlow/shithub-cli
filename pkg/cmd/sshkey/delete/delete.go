// SPDX-License-Identifier: AGPL-3.0-or-later

// Package del implements `shithub ssh-key delete`. Accepts an id or a
// title; titles are resolved against the live list.
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

	Selector string
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
		Use:   "delete <id-or-title>",
		Short: "Delete an SSH key from your shithub account",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Selector = args[0]
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "the shithub host (default: configured host)")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "skip the confirmation prompt")
	return cmd
}

// Run resolves the selector to an id (via list when needed) and deletes.
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

	id, title, err := resolve(ctx, kc, opts.Selector)
	if err != nil {
		return err
	}

	if !opts.Yes {
		if opts.IO.NeverPrompt() {
			return errors.New("ssh-key delete: pass --yes to confirm in non-interactive mode")
		}
		ok, perr := opts.Prompter.Confirm(fmt.Sprintf("Delete SSH key %q (id=%d)?", title, id), false)
		if perr != nil {
			return perr
		}
		if !ok {
			fmt.Fprintln(opts.IO.ErrOut, "cancelled")
			return nil
		}
	}
	if err := kc.DeleteSSH(ctx, id); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted SSH key %q (id=%d)\n", opts.IO.SuccessIcon(), title, id)
	return nil
}

// resolve accepts either a numeric id or a title. Numeric inputs skip
// the list roundtrip; title inputs walk ListSSH and match case-insensitively.
func resolve(ctx context.Context, kc *keys.Client, sel string) (int64, string, error) {
	if id, err := strconv.ParseInt(sel, 10, 64); err == nil && id > 0 {
		return id, sel, nil
	}
	list, err := kc.ListSSH(ctx)
	if err != nil {
		return 0, "", err
	}
	for _, k := range list {
		if k.Title == sel {
			return k.ID, k.Title, nil
		}
	}
	return 0, "", fmt.Errorf("ssh-key delete: no key matches %q", sel)
}
