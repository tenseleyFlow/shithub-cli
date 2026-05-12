// SPDX-License-Identifier: AGPL-3.0-or-later

// Package workflow wires the `shithub workflow` subtree onto root.
// list/view/run are fully implemented against shithub S41a–f. enable
// and disable are stubs deferred to S41g; they print a friendly
// "deferred" notice and exit 2 (same pattern as `shithub release`).
package workflow

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	workflowlist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/workflow/list"
	workflowrun "github.com/tenseleyFlow/shithub-cli/pkg/cmd/workflow/run"
	workflowview "github.com/tenseleyFlow/shithub-cli/pkg/cmd/workflow/view"
)

const deferredMessage = "workflow enable/disable not yet supported on this shithub host (see shithub-S41g)"

const deferredExitCode = 2

// exitFn is the indirection that lets tests substitute a non-fatal
// "exit" recorder. See pkg/cmd/release for the matching pattern.
var exitFn = os.Exit

// NewCmd builds the `workflow` parent.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow <command>",
		Short: "Inspect and trigger GitHub-Actions-style workflows",
	}
	cmd.AddCommand(workflowlist.NewCmd(f))
	cmd.AddCommand(workflowview.NewCmd(f))
	cmd.AddCommand(workflowrun.NewCmd(f))
	cmd.AddCommand(newDeferredStub("enable", "Enable a workflow (deferred until shithub-S41g)"))
	cmd.AddCommand(newDeferredStub("disable", "Disable a workflow (deferred until shithub-S41g)"))
	return cmd
}

func newDeferredStub(name, short string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <id-or-file>",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", cmd.CommandPath(), deferredMessage)
			exitFn(deferredExitCode)
			return nil
		},
	}
}
