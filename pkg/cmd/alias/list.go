// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type listOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
}

func newListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &listOptions{IO: f.IOStreams, Config: f.Config}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered aliases",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return listRun(c.Context(), opts)
		},
	}
	return cmd
}

func listRun(_ context.Context, opts *listOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if len(cfg.Aliases) == 0 {
		fmt.Fprintln(opts.IO.ErrOut, "no aliases configured")
		return nil
	}
	names := make([]string, 0, len(cfg.Aliases))
	for n := range cfg.Aliases {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(opts.IO.Out, "%s:\t%s\n", n, cfg.Aliases[n])
	}
	return nil
}
