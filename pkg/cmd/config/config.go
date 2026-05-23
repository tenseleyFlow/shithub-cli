// SPDX-License-Identifier: AGPL-3.0-or-later

// Package config implements `shithub config` — the user-facing wrapper
// around the internal/config substrate. Four verbs: get / set / list /
// clear-cache. Schema validation lives in the (key,value)-typed helpers
// so a bad value never lands on disk via this command.
package config

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewCmd returns the `config` parent command with every subcommand
// registered.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Read and write shithub-cli configuration",
		Long: `Manage entries in ~/.config/shithub/config.yml.

Recognized keys:
  git_protocol       'ssh' or 'https' (default: https)
  editor             command used for body composition
  browser            command used to open URLs
  pager              command used for paged output (empty disables)
  prompt             'enabled' or 'disabled'
  http_unix_socket   for shithub-over-unix dev only
  aliases.*          managed via 'shithub alias'

Per-host keys (passed with --host) currently cover only git_protocol;
everything else is global.`,
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(newGetCmd(f))
	cmd.AddCommand(newSetCmd(f))
	cmd.AddCommand(newListCmd(f))
	cmd.AddCommand(newClearCacheCmd(f))
	return cmd
}
