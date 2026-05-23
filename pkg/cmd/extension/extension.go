// SPDX-License-Identifier: AGPL-3.0-or-later

// Package extension wires the `shithub extension` subtree onto root.
// list/install/remove/upgrade/exec/create are fully implemented;
// search and browse are deferred stubs (see their package docs for
// the rationale).
package extension

import (
	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	extbrowse "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/browse"
	extcreate "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/create"
	extexec "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/exec"
	extinstall "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/install"
	extlist "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/list"
	extremove "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/remove"
	extsearch "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/search"
	extupgrade "github.com/tenseleyFlow/shithub-cli/pkg/cmd/extension/upgrade"
)

// NewCmd builds the `extension` parent.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extension <command>",
		Short: "Install and manage third-party shithub-cli extensions",
		Long: `Install and manage shithub-<verb> extensions.

Once installed, an extension named shithub-foo is invoked as ` + "`shithub foo`" + ` —
the host CLI looks up unknown verbs against ${SHITHUB_CONFIG_DIR}/extensions/
and execs the matching binary with arguments passed through.`,
		// I3: reject unknown subcommands; preserve help on bare invoke.
		Args: cobra.ArbitraryArgs,
		RunE: cmdutil.ParentRunE(),
	}
	cmd.AddCommand(extlist.NewCmd(f))
	cmd.AddCommand(extinstall.NewCmd(f))
	cmd.AddCommand(extremove.NewCmd(f))
	cmd.AddCommand(extupgrade.NewCmd(f))
	cmd.AddCommand(extexec.NewCmd(f))
	cmd.AddCommand(extcreate.NewCmd(f))
	cmd.AddCommand(extsearch.NewCmd(f))
	cmd.AddCommand(extbrowse.NewCmd(f))
	return cmd
}
