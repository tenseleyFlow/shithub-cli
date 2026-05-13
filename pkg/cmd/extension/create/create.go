// SPDX-License-Identifier: AGPL-3.0-or-later

// Package create implements `shithub extension create <name>`. Scaffolds
// a starter `shithub-<name>/` directory containing an executable script
// and a README, ready to commit + push.
package create

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/extension"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

type options struct {
	IO          *iostreams.IOStreams
	Name        string
	Precompiled string
	Force       bool

	// Root is where the scaffold is created. Empty = current working
	// directory.  Override for tests.
	Root string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Scaffold a new shithub-cli extension",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.Name = args[0]
			return Run(c.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Precompiled, "precompiled", "", "scaffold a compiled-language template (deferred — script only this sprint)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "overwrite an existing shithub-<name> directory")
	return cmd
}

// Run scaffolds.
func Run(_ context.Context, opts *options) error {
	if opts.Precompiled != "" {
		// The --precompiled flag is registered for forward compat with
		// gh; until we ship the Go boilerplate it's a soft-deferred
		// error rather than a silent ignore so users know what they
		// asked for didn't happen.
		return errors.New("extension create: --precompiled is not yet implemented (script-only scaffolds this sprint)")
	}
	root := opts.Root
	if root == "" {
		root = "."
	}
	dir, err := extension.Scaffold(root, opts.Name, opts.Force)
	if err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Scaffolded extension at %s\n", opts.IO.SuccessIcon(), dir)
	fmt.Fprintf(opts.IO.ErrOut, "Next steps:\n")
	fmt.Fprintf(opts.IO.ErrOut, "  cd %s\n", dir)
	fmt.Fprintf(opts.IO.ErrOut, "  git init && git add . && git commit -m 'init'\n")
	fmt.Fprintf(opts.IO.ErrOut, "  shithub repo create <owner>/shithub-%s --push\n", opts.Name)
	return nil
}
