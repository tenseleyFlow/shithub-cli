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

type getOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
	Hosts  func() (config.Hosts, error)

	Key  string
	Host string
}

func newGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &getOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		Hosts:  f.Hosts,
	}
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print the current value of a config key",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Key = args[0]
			return getRun(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Host, "host", "", "read the value scoped to this host instead of the global config")
	return cmd
}

func getRun(_ context.Context, opts *getOptions) error {
	if opts.Host != "" {
		return getHostScoped(opts)
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	value, set, err := readKey(cfg, opts.Key)
	if err != nil {
		return err
	}
	if !set {
		// Empty stdout, non-zero exit so scripts can detect.
		return fmt.Errorf("config: key %q is unset", opts.Key)
	}
	fmt.Fprintln(opts.IO.Out, value)
	return nil
}

func getHostScoped(opts *getOptions) error {
	hosts, err := opts.Hosts()
	if err != nil {
		return err
	}
	host := config.NormalizeHost(opts.Host)
	entry, ok := hosts[host]
	if !ok {
		return fmt.Errorf("config: not authenticated to %s", host)
	}
	value, set, err := readHostKey(entry, opts.Key)
	if err != nil {
		return err
	}
	if !set {
		return fmt.Errorf("config: host %s has no %q set", host, opts.Key)
	}
	fmt.Fprintln(opts.IO.Out, value)
	return nil
}
