// SPDX-License-Identifier: AGPL-3.0-or-later

// Package label is the parent for the `shithub label` subcommand tree.
package label

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	cloneCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/label/clone"
	createCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/label/create"
	deleteCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/label/delete"
	editCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/label/edit"
	listCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/label/list"
)

// NewCmd builds the `shithub label` parent and registers its subcommands.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label <command> [flags]",
		Short: "Manage repository labels",
		Long: `Work with repository labels.

Common subcommands:
  list    list labels
  create  create a new label
  edit    edit a label (rename / color / description)
  delete  delete a label
  clone   clone labels from another repository
`,
	}
	cmd.AddCommand(listCmd.NewCmd(f))
	cmd.AddCommand(createCmd.NewCmd(f))
	cmd.AddCommand(editCmd.NewCmd(f))
	cmd.AddCommand(deleteCmd.NewCmd(f))
	cmd.AddCommand(cloneCmd.NewCmd(f))
	return cmd
}
