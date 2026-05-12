// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type setOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
	Hosts  func() (config.Hosts, error)

	Key, Value string
	Host       string
}

func newSetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &setOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		Hosts:  f.Hosts,
	}
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Update a config key",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Key, opts.Value = args[0], args[1]
			return setRun(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Host, "host", "", "scope this update to a single host's hosts.yml entry")
	return cmd
}

func setRun(_ context.Context, opts *setOptions) error {
	if opts.Host != "" {
		return setHostScoped(opts)
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if err := writeKey(cfg, opts.Key, opts.Value); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("config: save: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %s set to %q\n", opts.IO.SuccessIcon(), opts.Key, opts.Value)
	return nil
}

func setHostScoped(opts *setOptions) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	host, err := config.ValidateHost(opts.Host)
	if err != nil {
		return err
	}
	entry, ok := hosts[host]
	if !ok {
		return fmt.Errorf("config: not authenticated to %s; run 'shithub auth login --hostname %s' first", host, host)
	}
	if err := writeHostKey(entry, opts.Key, opts.Value); err != nil {
		return err
	}
	if err := hosts.Save(); err != nil {
		return fmt.Errorf("config: save hosts: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s %s set to %q for %s\n", opts.IO.SuccessIcon(), opts.Key, opts.Value, host)
	return nil
}
