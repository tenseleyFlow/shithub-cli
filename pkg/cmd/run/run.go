// SPDX-License-Identifier: AGPL-3.0-or-later

// Package run wires the `shithub run` subtree onto root. list, view,
// watch, and download read against shithub S41a–f. rerun, cancel, and
// delete are stubs deferred to S41g — same friendly-defer pattern used
// by `shithub release` and `shithub workflow`.
package run

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	rundownload "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/download"
	runlist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/list"
	runview "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/view"
	runwatch "github.com/tenseleyFlow/shithub-cli/pkg/cmd/run/watch"
)

// NewCmd builds the `run` parent. H4: rerun/cancel/delete stubs
// migrated to shared cmdutil.NewDeferredCmd factory.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <command>",
		Short: "Inspect workflow runs and their artifacts",
	}
	cmd.AddCommand(runlist.NewCmd(f))
	cmd.AddCommand(runview.NewCmd(f))
	cmd.AddCommand(runwatch.NewCmd(f))
	cmd.AddCommand(rundownload.NewCmd(f))
	stub := func(name, short string) *cobra.Command {
		return cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
			Use: name + " <run-id>", Short: short,
			Name: "shithub run " + name, Track: "shithub-S41g",
		})
	}
	cmd.AddCommand(stub("rerun", "Rerun a workflow run (deferred until shithub-S41g)"))
	cmd.AddCommand(stub("cancel", "Cancel an in-progress run (deferred until shithub-S41g)"))
	cmd.AddCommand(stub("delete", "Delete a workflow run (deferred until shithub-S41g)"))
	return cmd
}
