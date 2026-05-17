// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestEveryCommandCanInitHelpFlag walks rootCmd's full subtree and
// invokes cobra's InitDefaultHelpFlag on each node. That call panics
// at runtime if a custom flag has already claimed the "h" shorthand
// that cobra reserves for --help — which is exactly the failure mode
// `shithub repo create` shipped with (audit finding A10): a custom
// --homepage flag with shorthand "h" made the command unusable on
// every invocation.
//
// Unit tests for individual create/edit Run() paths bypass NewCmd and
// so could never catch this. This test executes the same code path
// cobra's runtime takes, so any future -h collision lands here
// instead of in a user's terminal.
func TestEveryCommandCanInitHelpFlag(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		// InitDefaultHelpFlag panics on shorthand collision; let the
		// test framework surface the panic as a failure with the
		// offending command path.
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("InitDefaultHelpFlag panic on %q: %v",
						c.CommandPath(), r)
				}
			}()
			c.InitDefaultHelpFlag()
		}()
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
}
