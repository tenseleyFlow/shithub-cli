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
		// H24: when the expansion begins with `--` (e.g. `alias set
		// x "--help"`), cobra ate it as a flag for `set` itself —
		// help printed, alias never registered, exit 0. With
		// DisableFlagParsing we parse positionals ourselves and
		// surface `--shell` via a strings.Contains scan; flag-like
		// expansions now round-trip cleanly. The audit's workaround
		// `set name -- --help` still works because we treat `--` as
		// a positional separator.
		DisableFlagParsing: true,
		RunE: func(c *cobra.Command, args []string) error {
			name, expansion, shell, err := parseSetArgs(args)
			if err != nil {
				return err
			}
			opts.Name = name
			opts.Expansion = expansion
			opts.Shell = shell
			opts.BuiltinNames = builtinNames(c.Root())
			return setRun(c.Context(), opts)
		},
	}
	// Declared so `--shell` shows up in help even though parsing is
	// off; the actual value comes from parseSetArgs.
	cmd.Flags().BoolVar(&opts.Shell, "shell", false, "treat the expansion as a shell command (prepends '!')")
	return cmd
}

// parseSetArgs extracts (name, expansion, shell) from the verbatim
// argv handed to `alias set`. We pull `--shell` if it appears among
// the positionals; everything else lands in the expansion. A leading
// `--` is consumed as a separator so the audited workaround
// `set name -- --help` still works.
func parseSetArgs(args []string) (name, expansion string, shell bool, err error) {
	rest := make([]string, 0, len(args))
	sawSep := false
	for _, a := range args {
		if !sawSep && a == "--" {
			sawSep = true
			continue
		}
		if !sawSep && a == "--shell" {
			shell = true
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) != 2 {
		return "", "", false, fmt.Errorf("alias set: expected exactly two positional args (got %d); pass --shell as a flag, and use `--` to separate flag-shaped expansions like `alias set x -- --help`", len(rest))
	}
	return rest[0], rest[1], shell, nil
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
