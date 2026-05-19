// SPDX-License-Identifier: AGPL-3.0-or-later

// Package deploykey registers the `shithub repo deploy-key` subtree as
// a deferred surface. shithub's server doesn't have the per-repo
// deploy-key endpoints yet; the CLI ships discoverable stubs so users
// who copy gh-style commands see a friendly notice instead of an
// "unknown subcommand" silent success (E-audit E12).
//
// Riding cmdutil.NewDeferredCmd means the stubs accept arbitrary flags
// (E20) and always carry --repo/-R (E21).
package deploykey

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

const track = "S50 §X repo deploy keys"

// NewCmd builds the parent and registers the deferred children.
func NewCmd(_ *cmdutil.Factory) *cobra.Command {
	parent := &cobra.Command{
		Use:   "deploy-key <command>",
		Short: "Manage deploy keys (deferred — server-side support pending)",
		Args:  cobra.ArbitraryArgs,
		// No-arg prints help; an unknown verb errors so the parent
		// itself doesn't silently exit 0 (sibling of E16).
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) == 0 {
				return c.Help()
			}
			return fmt.Errorf("unknown command %q for %q; run '%s --help' for usage",
				args[0], c.CommandPath(), c.CommandPath())
		},
	}
	parent.AddCommand(cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
		Use:   "list",
		Short: "List a repo's deploy keys (deferred)",
		Name:  "repo deploy-key list",
		Track: track,
	}))
	parent.AddCommand(cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
		Use:   "add <key-file>",
		Short: "Add a deploy key to a repo (deferred)",
		Name:  "repo deploy-key add",
		Track: track,
	}))
	parent.AddCommand(cmdutil.NewDeferredCmd(cmdutil.DeferredSpec{
		Use:   "delete <id>",
		Short: "Delete a deploy key (deferred)",
		Name:  "repo deploy-key delete",
		Track: track,
	}))
	return parent
}
