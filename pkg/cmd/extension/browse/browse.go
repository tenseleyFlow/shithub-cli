// SPDX-License-Identifier: AGPL-3.0-or-later

// Package browse implements `shithub extension browse` as a deferred
// stub. A bubbletea-backed TUI is the intended implementation; pulling
// in a heavy TUI dependency without exploring the rest of the
// interactive surface first would be premature.
package browse

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
)

const deferredMessage = "extension browse is not yet implemented (bubbletea TUI deferred — use `extension list` for now)"

// ExitFn lets tests substitute a recorder.
var ExitFn = os.Exit

type options struct {
	IO *iostreams.IOStreams
}

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &options{IO: f.IOStreams}
	cmd := &cobra.Command{
		Use:   "browse",
		Short: "Browse installed extensions in a TUI (deferred)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return Run(c.Context(), opts)
		},
	}
	return cmd
}

// Run prints the deferred notice and exits 2.
func Run(_ context.Context, opts *options) error {
	fmt.Fprintln(opts.IO.ErrOut, "extension browse:", deferredMessage)
	ExitFn(2)
	return nil
}
