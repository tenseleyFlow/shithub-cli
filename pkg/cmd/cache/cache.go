// SPDX-License-Identifier: AGPL-3.0-or-later

// Package cache wires the `shithub cache` subtree onto root. list reads
// against shithub S41f; delete is a stub deferred to S41g.
package cache

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	cachelist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/cache/list"
)

const deferredMessage = "cache delete not yet supported on this shithub host (see shithub-S41g)"

const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder. See pkg/cmd/release for the matching pattern.
var exitFn = os.Exit

// NewCmd builds the `cache` parent.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache <command>",
		Short: "Manage GitHub-Actions-compatible cache entries",
		// C-audit C18: without an explicit Args/RunE on the parent,
		// `shithub cache totally-not-a-real-subcmd` printed the
		// parent's help and exited 0. CI scripts running an unknown
		// subcommand against an older binary silently no-op'd. Reject
		// any positional that didn't match a registered subcommand.
		Args: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown subcommand %q for %q", args[0], c.CommandPath())
			}
			return nil
		},
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}
	cmd.AddCommand(cachelist.NewCmd(f))
	cmd.AddCommand(newDeferredStub("delete", "Delete a cache entry (deferred until shithub-S41g)"))
	return cmd
}

func newDeferredStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <id-or-key>",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
			exitFn(deferredExitCode)
			return nil
		},
	}
}
