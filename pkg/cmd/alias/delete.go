// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type deleteOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)

	Name string
	All  bool
}

func newDeleteCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &deleteOptions{IO: f.IOStreams, Config: f.Config}
	cmd := &cobra.Command{
		Use:   "delete [<name>]",
		Short: "Remove an alias (or every alias with --all)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.Name = args[0]
			}
			return deleteRun(c.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.All, "all", false, "delete every registered alias")
	return cmd
}

func deleteRun(_ context.Context, opts *deleteOptions) error {
	if !opts.All && opts.Name == "" {
		return errors.New("alias delete: pass an alias name or --all")
	}
	if opts.All && opts.Name != "" {
		return errors.New("alias delete: --all and <name> are mutually exclusive")
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}

	if opts.All {
		removed := len(cfg.Aliases)
		cfg.Aliases = map[string]string{}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("alias delete: save: %w", err)
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s removed %d alias(es)\n", opts.IO.SuccessIcon(), removed)
		return nil
	}

	if _, ok := cfg.Aliases[opts.Name]; !ok {
		return fmt.Errorf("alias delete: no alias %q", opts.Name)
	}
	delete(cfg.Aliases, opts.Name)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("alias delete: save: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s alias %s removed\n", opts.IO.SuccessIcon(), opts.Name)
	return nil
}
