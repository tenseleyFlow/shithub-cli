// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutiltest

import (
	"testing"

	"github.com/spf13/cobra"
)

// RunCobra invokes a cobra command end-to-end against the test
// Factory's IO buffers. Use this in roundtrip tests so the command
// goes through cobra's flag parsing — the layer where wire-name bugs
// like F-audit F11 (`creator` vs `author`) hide when tests call the
// internal Run() function directly.
//
// The returned error is whatever cobra surfaced from Execute (or
// SilenceErrors/SilenceUsage didn't suppress). Tests typically:
//
//	err := tf.RunCobra(t, list.NewCmd(tf.Factory), "--author", "ghost", "-R", "alice/demo")
//	tf.Server.AssertQueryParam(http.MethodGet, "/api/v1/repos/alice/demo/issues", "author", "ghost")
//
// Output is captured in tf.Out / tf.ErrOut as usual; the writers are
// rebound to the command so anything it prints lands there.
func (tf *Factory) RunCobra(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	cmd.SetArgs(args)
	cmd.SetOut(tf.Out)
	cmd.SetErr(tf.ErrOut)
	cmd.SetIn(tf.In)
	// Keep usage out of test logs; SilenceErrors lets the test inspect
	// the returned error rather than seeing cobra echo it to stderr.
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	return cmd.Execute()
}
