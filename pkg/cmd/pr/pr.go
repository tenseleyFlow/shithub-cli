// SPDX-License-Identifier: AGPL-3.0-or-later

// Package pr is the parent for the `shithub pr` subcommand tree.
package pr

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	checkoutCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/checkout"
	checksCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/checks"
	closeCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/close"
	commentCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/comment"
	createCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/create"
	diffCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/diff"
	editCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/edit"
	listCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/list"
	lockCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/lock"
	mergeCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/merge"
	readyCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/ready"
	reopenCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/reopen"
	reviewCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/review"
	statusCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/status"
	updatebranchCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/updatebranch"
	viewCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/view"
)

// NewCmd builds the `shithub pr` parent and registers its subcommands.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command> [flags]",
		Short: "Manage pull requests",
		Long: `Work with pull requests on shithub.

Common subcommands:
  create        open a new pull request
  list          list pull requests
  view          view a single pull request
  status        show PRs relevant to you across repos
  checkout      check out a pull request locally
  diff          show the diff
  edit          edit title/body/base/labels/assignees/reviewers
  close         close a pull request
  reopen        reopen a closed pull request
  ready         mark a draft as ready for review (--undo for the reverse)
  update-branch update the PR branch with its base
  lock          lock the conversation
  unlock        unlock the conversation
  review        submit a review (approve / request-changes / comment)
  comment       add or edit a comment on the PR conversation
  merge         merge a pull request (merge/squash/rebase, auto, admin)
  checks        show check runs for the PR head SHA
`,
	}
	cmd.AddCommand(createCmd.NewCmd(f))
	cmd.AddCommand(listCmd.NewCmd(f))
	cmd.AddCommand(viewCmd.NewCmd(f))
	cmd.AddCommand(statusCmd.NewCmd(f))
	cmd.AddCommand(checkoutCmd.NewCmd(f))
	cmd.AddCommand(diffCmd.NewCmd(f))
	cmd.AddCommand(editCmd.NewCmd(f))
	cmd.AddCommand(closeCmd.NewCmd(f))
	cmd.AddCommand(reopenCmd.NewCmd(f))
	cmd.AddCommand(readyCmd.NewCmd(f))
	cmd.AddCommand(updatebranchCmd.NewCmd(f))
	cmd.AddCommand(lockCmd.NewLockCmd(f))
	cmd.AddCommand(lockCmd.NewUnlockCmd(f))
	cmd.AddCommand(reviewCmd.NewCmd(f))
	cmd.AddCommand(commentCmd.NewCmd(f))
	cmd.AddCommand(mergeCmd.NewCmd(f))
	cmd.AddCommand(checksCmd.NewCmd(f))
	return cmd
}
