// SPDX-License-Identifier: AGPL-3.0-or-later

// Package exec implements `shithub extension exec <name> -- <args...>`.
// Useful when the user doesn't want to rely on the unknown-verb
// dispatcher; also lets scripts re-enter the host CLI explicitly.
package exec

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/extension"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type options struct {
	IO   *iostreams.IOStreams
	Name string
	Args []string
	Dir  string // override for tests

	exitFn func(int) // override for tests
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams, exitFn: os.Exit}
	cmd := &cobra.Command{
		Use:                "exec <name> [-- args...]",
		Short:              "Run an installed extension by name",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true, // forward all flags verbatim to the extension
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			opts.Args = args[1:]
			return Run(c.Context(), opts)
		},
	}
	return cmd
}

// Run looks up the extension and execs it.
func Run(_ context.Context, opts *options) error {
	if err := extension.ValidateName(opts.Name); err != nil {
		return err
	}
	dir, err := resolveDir(opts.Dir)
	if err != nil {
		return err
	}
	path, ok := extension.Find(dir, opts.Name)
	if !ok {
		return fmt.Errorf("extension exec: %q is not installed (`shithub extension install`)", opts.Name)
	}
	opts.exitFn(extension.Exec(path, opts.Args))
	return nil
}

func resolveDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.ExtensionsDir()
}
