// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ParentRunE returns a RunE suitable for a parent/grouping command that
// only exists to host subcommands. With no args we print the parent
// help (like gh's `gh org` does, exit 0). With an unrecognized arg we
// return an error so the binary exits non-zero — cobra's default routes
// the call through the parent's help text and exits 0, which made
// typos like `shithub org INVALID` falsely look successful (E-audit E16).
//
// gh emits "unknown command \"INVALID\" for \"gh org\""; we mirror that
// shape and let the user discover the available subcommands via the
// suggestion line cobra appends automatically.
func ParentRunE() func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		return fmt.Errorf("unknown command %q for %q; run '%s --help' for usage",
			args[0], c.CommandPath(), c.CommandPath())
	}
}
