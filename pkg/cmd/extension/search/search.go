// SPDX-License-Identifier: AGPL-3.0-or-later

// Package search implements `shithub extension search <query>` as a
// deferred stub. The intended implementation runs a topic-filtered
// search for repos tagged `shithub-extension`, but topic search isn't
// firm on the shithub backend yet — wiring it before the server has
// committed to topic indexing risks promising a UX we can't deliver.
package search

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

const deferredMessage = "extension search is not yet supported (topic indexing pending on the shithub server side); use `shithub search repos --include-topics` once topic search lands"

// ExitFn lets tests substitute a recorder. Exported so the parent's
// test can override across the whole subtree if it wants to.
var ExitFn = os.Exit

type options struct {
	IO    *iostreams.IOStreams
	Query string
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for shithub-cli extensions (deferred)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Query = args[0]
			}
			return Run(c.Context(), opts)
		},
	}
	return cmd
}

// Run prints the deferred notice and exits 2.
func Run(_ context.Context, opts *options) error {
	fmt.Fprintln(opts.IO.ErrOut, "extension search:", deferredMessage)
	ExitFn(2)
	return nil
}
