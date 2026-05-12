// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import "github.com/spf13/cobra"

// builtinNames returns the names of every top-level command currently
// mounted on the root. Used to reject alias `set` calls that would
// shadow a built-in. The walk is one level deep — aliases shadow
// top-level verbs, never sub-verbs.
func builtinNames(root *cobra.Command) []string {
	if root == nil {
		return nil
	}
	out := make([]string, 0, len(root.Commands()))
	for _, c := range root.Commands() {
		// Hidden commands (e.g., the git-credential helper) count too: we
		// don't want an alias to silently mask one.
		out = append(out, c.Name())
	}
	return out
}
