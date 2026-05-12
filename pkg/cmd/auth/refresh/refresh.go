// SPDX-License-Identifier: AGPL-3.0-or-later

// Package refresh stubs `shithub auth refresh`. Full OAuth-device-flow
// re-authentication lands in C04a once the shithub server ships the
// device-code endpoints. Today the command exists to reserve the verb
// and emit a clear, actionable message so users don't get a "command
// not found" surprise.
package refresh

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// NewCmd builds the cobra command.
func NewCmd(f *cmdutil.Factory) *cobra.Command {
	ios := f.IOStreams
	cmd := &cobra.Command{
		Use:    "refresh",
		Short:  "Re-authenticate to expand or rotate scopes (deferred to C04a)",
		Args:   cobra.NoArgs,
		Hidden: false, // visible so users discover the gap and the workaround
		RunE: func(c *cobra.Command, _ []string) error {
			fmt.Fprintln(ios.ErrOut,
				ios.WarningIcon()+" auth refresh is not yet supported on this build.\n"+
					"  Re-authenticate with: shithub auth login --with-token < new-token.txt\n"+
					"  Tracked in: shithub-cli sprint C04a (OAuth device flow).")
			return fmt.Errorf("auth refresh: not yet supported")
		},
	}
	return cmd
}
