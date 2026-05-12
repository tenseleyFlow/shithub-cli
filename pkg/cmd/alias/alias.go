// SPDX-License-Identifier: AGPL-3.0-or-later

// Package alias implements `shithub alias` — the user-facing CRUD for
// alias entries persisted in config.yml. The expansion engine lives in
// internal/alias; this package handles the cobra wiring + persistence
// gates (built-in collision check, name validation).
package alias

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewCmd returns the `alias` parent command with subcommands registered.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias <command>",
		Short: "Create command shortcuts",
		Long: `Aliases give shorthand names to longer command invocations.

Plain aliases:    shithub alias set co "pr checkout"
Shell aliases:    shithub alias set --shell mine '!shithub pr list --author @me'
Bulk import:      shithub alias import aliases.yml [--clobber]

Aliases never shadow built-in commands; the dispatcher always prefers
the built-in if both exist.`,
	}
	cmd.AddCommand(newSetCmd(f))
	cmd.AddCommand(newListCmd(f))
	cmd.AddCommand(newDeleteCmd(f))
	cmd.AddCommand(newImportCmd(f))
	return cmd
}
