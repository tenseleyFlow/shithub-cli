// SPDX-License-Identifier: AGPL-3.0-or-later

// Package issue is the parent for the `shithub issue` subcommand tree.
package issue

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	closeCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/close"
	commentCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/comment"
	createCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/create"
	deleteCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/delete"
	editCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/edit"
	listCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/list"
	lockCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/lock"
	reopenCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/reopen"
	statusCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/status"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/stubs"
	viewCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/issue/view"
)

// NewCmd builds the `shithub issue` parent and registers its subcommands.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <command> [flags]",
		Short: "Manage issues",
		Long: `Work with issues on shithub.

Common subcommands:
  create    create a new issue
  list      list issues
  view      view a single issue
  status    show issues relevant to you across repos
  close     close an issue
  reopen    reopen a closed issue
  edit      edit issue title/body/labels/assignees/milestone
  comment   add or edit a comment
  delete    delete an issue
  lock      lock an issue
  unlock    unlock an issue
  pin       pin an issue (server-side support pending)
  unpin     unpin an issue (server-side support pending)
  transfer  transfer to another repo (server-side support pending)
  develop   create/link a branch to an issue (server-side support pending)
`,
		// E-audit E16: unknown subcommand → exit non-zero.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(createCmd.NewCmd(f))
	cmd.AddCommand(listCmd.NewCmd(f))
	cmd.AddCommand(viewCmd.NewCmd(f))
	cmd.AddCommand(statusCmd.NewCmd(f))
	cmd.AddCommand(closeCmd.NewCmd(f))
	cmd.AddCommand(reopenCmd.NewCmd(f))
	cmd.AddCommand(editCmd.NewCmd(f))
	cmd.AddCommand(commentCmd.NewCmd(f))
	cmd.AddCommand(deleteCmd.NewCmd(f))
	cmd.AddCommand(lockCmd.NewLockCmd(f))
	cmd.AddCommand(lockCmd.NewUnlockCmd(f))
	cmd.AddCommand(stubs.NewPinCmd(f))
	cmd.AddCommand(stubs.NewUnpinCmd(f))
	cmd.AddCommand(stubs.NewTransferCmd(f))
	cmd.AddCommand(stubs.NewDevelopCmd(f))
	return cmd
}
