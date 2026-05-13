// SPDX-License-Identifier: AGPL-3.0-or-later

// Package list implements `shithub extension list`.
package list

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/extension"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
)

type options struct {
	IO  *iostreams.IOStreams
	Dir string // override for tests
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed extensions",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	return cmd
}

// Run reads the extensions dir and renders the listing.
func Run(_ context.Context, opts *options) error {
	dir, err := resolveDir(opts.Dir)
	if err != nil {
		return err
	}
	xs, err := extension.List(dir)
	if err != nil {
		return err
	}
	if len(xs) == 0 {
		fmt.Fprintln(opts.IO.ErrOut, "no extensions installed")
		return nil
	}
	tp := tableprinter.New(opts.IO.Out, opts.IO.IsStdoutTTY(), opts.IO.TerminalWidth())
	for _, x := range xs {
		source := "manual"
		if _, err := os.Stat(filepath.Join(x.Path, ".git")); err == nil {
			source = "git"
		}
		tp.AddRow(x.Name, x.Path, source)
	}
	_ = tp.Render()
	return nil
}

func resolveDir(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.ExtensionsDir()
}
