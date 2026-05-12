// SPDX-License-Identifier: AGPL-3.0-or-later

// Package variable wires the `shithub variable` subtree onto root.
package variable

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	variabledelete "github.com/tenseleyFlow/shithub-cli/pkg/cmd/variable/delete"
	variableget "github.com/tenseleyFlow/shithub-cli/pkg/cmd/variable/get"
	variablelist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/variable/list"
	variableset "github.com/tenseleyFlow/shithub-cli/pkg/cmd/variable/set"
)

// NewCmd builds the parent.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "variable <command>",
		Short: "Manage GitHub-Actions-style variables",
	}
	cmd.AddCommand(variablelist.NewCmd(f))
	cmd.AddCommand(variableget.NewCmd(f))
	cmd.AddCommand(variableset.NewCmd(f))
	cmd.AddCommand(variabledelete.NewCmd(f))
	return cmd
}
