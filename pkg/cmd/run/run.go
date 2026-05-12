// SPDX-License-Identifier: AGPL-3.0-or-later

// Package run wires the `shithub run` subtree onto root. list, view,
// watch, and download read against shithub S41a–f. rerun, cancel, and
// delete are stubs deferred to S41g — same friendly-defer pattern used
// by `shithub release` and `shithub workflow`.
package run

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	rundownload "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/download"
	runlist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/list"
	runview "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/view"
	runwatch "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/watch"
)

const deferredMessage = "run rerun/cancel/delete not yet supported on this shithub host (see shithub-S41g)"

const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder. See pkg/cmd/release for the matching pattern.
var exitFn = os.Exit

// NewCmd builds the `run` parent.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <command>",
		Short: "Inspect workflow runs and their artifacts",
	}
	cmd.AddCommand(runlist.NewCmd(f))
	cmd.AddCommand(runview.NewCmd(f))
	cmd.AddCommand(runwatch.NewCmd(f))
	cmd.AddCommand(rundownload.NewCmd(f))
	cmd.AddCommand(newDeferredStub("rerun", "Rerun a workflow run (deferred until shithub-S41g)"))
	cmd.AddCommand(newDeferredStub("cancel", "Cancel an in-progress run (deferred until shithub-S41g)"))
	cmd.AddCommand(newDeferredStub("delete", "Delete a workflow run (deferred until shithub-S41g)"))
	return cmd
}

func newDeferredStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <run-id>",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
			exitFn(deferredExitCode)
			return nil
		},
	}
}
