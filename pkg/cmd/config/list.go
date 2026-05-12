// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type listOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)

	JSON bool
}

func newListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &listOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured keys",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return listRun(c.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "emit machine-readable JSON")
	return cmd
}

func listRun(_ context.Context, opts *listOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	keys := listKeys(cfg)

	if opts.JSON {
		out := map[string]string{}
		for _, k := range keys {
			v, _, _ := readKey(cfg, k)
			out[k] = v
		}
		enc := json.NewEncoder(opts.IO.Out)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	for _, k := range keys {
		v, _, err := readKey(cfg, k)
		if err != nil {
			continue
		}
		fmt.Fprintf(opts.IO.Out, "%s=%s\n", k, v)
	}
	return nil
}
