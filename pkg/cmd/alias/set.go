// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/alias"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type setOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)

	Name      string
	Expansion string
	Shell     bool

	BuiltinNames []string
}

func newSetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &setOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}
	cmd := &cobra.Command{
		Use:   "set <name> <expansion>",
		Short: "Register a new alias",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.Expansion = args[1]
			opts.BuiltinNames = builtinNames(c.Root())
			return setRun(c.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Shell, "shell", false, "treat the expansion as a shell command (prepends '!')")
	return cmd
}

func setRun(_ context.Context, opts *setOptions) error {
	if err := alias.Validate(opts.Name, opts.BuiltinNames); err != nil {
		return err
	}

	expansion := opts.Expansion
	if opts.Shell && !strings.HasPrefix(expansion, alias.ShellPrefix) {
		expansion = alias.ShellPrefix + expansion
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	if cfg.Aliases == nil {
		cfg.Aliases = map[string]string{}
	}
	// C26: warn on overwrite so users notice when they typo over an
	// existing alias. gh prints `! Changing alias X from Y to Z`.
	if prev, ok := cfg.Aliases[opts.Name]; ok && prev != expansion {
		fmt.Fprintf(opts.IO.ErrOut, "! Changing alias %s from %s to %s\n", opts.Name, prev, expansion)
	}
	cfg.Aliases[opts.Name] = expansion
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("alias: save: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s alias %s set to %s\n", opts.IO.SuccessIcon(), opts.Name, expansion)
	return nil
}
