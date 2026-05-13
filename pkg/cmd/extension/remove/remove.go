// SPDX-License-Identifier: AGPL-3.0-or-later

// Package remove implements `shithub extension remove <name>` (with
// `uninstall` as an alias to match gh).
package remove

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/extension"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type options struct {
	IO   *iostreams.IOStreams
	Name string
	Dir  string // override for tests
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"uninstall"},
		Short:   "Remove an installed extension",
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			return Run(c.Context(), opts)
		},
	}
	return cmd
}

// Run deletes the extension dir.
func Run(_ context.Context, opts *options) error {
	if err := extension.ValidateName(opts.Name); err != nil {
		return err
	}
	dir, err := resolveDir(opts.Dir)
	if err != nil {
		return err
	}
	target := filepath.Join(dir, extension.Prefix+opts.Name)
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("extension remove: %q is not installed", opts.Name)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("extension remove: %w", err)
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Removed extension %q\n", opts.IO.SuccessIcon(), opts.Name)
	return nil
}

func resolveDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.ExtensionsDir()
}
