// SPDX-License-Identifier: AGPL-3.0-or-later

// Package org wires the `shithub org` subtree onto the root command.
// gh's org surface is intentionally small (list + view); we mirror it.
package org

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	orglist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/org/list"
	orgview "github.com/tenseleyFlow/shithub-cli/pkg/cmd/org/view"
)

// NewCmd builds the parent org command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org <command>",
		Short: "Work with organizations",
		// E-audit E16: error on unknown subcommand instead of cobra's
		// silent help+exit-0 default.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(orglist.NewCmd(f))
	cmd.AddCommand(orgview.NewCmd(f))
	return cmd
}
