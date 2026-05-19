// SPDX-License-Identifier: AGPL-3.0-or-later

// Package repo is the parent for the `shithub repo` subcommand tree.
// Subcommands live in sibling packages; this file just stitches them
// together so the root command file (cmd/shithub/root.go) registers a
// single node.
package repo

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	archiveCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/archive"
	cloneCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/clone"
	createCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/create"
	deleteCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/delete"
	deploykeyCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/deploykey"
	editCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/edit"
	forkCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/fork"
	listCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/list"
	renameCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/rename"
	setdefaultCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/setdefault"
	syncCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/sync"
	viewCmd "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/view"
)

// NewCmd builds the `shithub repo` parent and registers its subcommands.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <command> [flags]",
		Short: "Manage repositories",
		Long: `Work with shithub repositories.

Common subcommands:
  view      view a repository
  list      list repositories
  create    create a new repository
  clone     clone a repository locally
  fork      create a fork of a repository
  edit      edit repository settings
  rename    rename a repository
  archive   archive a repository
  unarchive unarchive a repository
  delete    delete a repository
  set-default set the default repository for the working directory
  sync      sync a fork with its upstream
  deploy-key manage deploy keys (deferred — server-side support pending)
`,
	}
	cmd.AddCommand(viewCmd.NewCmd(f))
	cmd.AddCommand(listCmd.NewCmd(f))
	cmd.AddCommand(createCmd.NewCmd(f))
	cmd.AddCommand(cloneCmd.NewCmd(f))
	cmd.AddCommand(forkCmd.NewCmd(f))
	cmd.AddCommand(editCmd.NewCmd(f))
	cmd.AddCommand(renameCmd.NewCmd(f))
	cmd.AddCommand(archiveCmd.NewArchiveCmd(f))
	cmd.AddCommand(archiveCmd.NewUnarchiveCmd(f))
	cmd.AddCommand(deleteCmd.NewCmd(f))
	cmd.AddCommand(setdefaultCmd.NewCmd(f))
	cmd.AddCommand(syncCmd.NewCmd(f))
	cmd.AddCommand(deploykeyCmd.NewCmd(f))
	return cmd
}
