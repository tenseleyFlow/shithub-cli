// SPDX-License-Identifier: AGPL-3.0-or-later

// Package workflow wires the `shithub workflow` subtree onto root.
// list/view/run are fully implemented against shithub S41a–f. enable
// and disable are stubs deferred to S41g; they print a friendly
// "deferred" notice and exit 2 (same pattern as `shithub release`).
package workflow

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	workflowlist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/workflow/list"
	workflowrun "github.com/tenseleyFlow/shithub-cli/pkg/cmd/workflow/run"
	workflowview "github.com/tenseleyFlow/shithub-cli/pkg/cmd/workflow/view"
)

// NewCmd builds the `workflow` parent. H4: enable/disable migrated to
// shared cmdutil.NewDeferredCmd factory.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow <command>",
		Short: "Inspect and trigger GitHub-Actions-style workflows",
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(workflowlist.NewCmd(f))
	cmd.AddCommand(workflowview.NewCmd(f))
	cmd.AddCommand(workflowrun.NewCmd(f))
	stub := func(name, short string) *cobra.Command {
		return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
			Use: name + " <id-or-file>", Short: short,
			Name: "shithub workflow " + name, Track: "shithub-S41g",
		})
	}
	cmd.AddCommand(stub("enable", "Enable a workflow (deferred until shithub-S41g)"))
	cmd.AddCommand(stub("disable", "Disable a workflow (deferred until shithub-S41g)"))
	return cmd
}
