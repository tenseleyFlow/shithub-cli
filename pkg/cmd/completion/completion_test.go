// SPDX-License-Identifier: AGPL-3.0-or-later

package completion

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// buildRootWithCompletion stitches a minimal cobra tree so completion
// scripts have something to generate against.
func buildRootWithCompletion(t *testing.T) (*cobra.Command, *cmdutiltest.Factory) {
	t.Helper()
	tf := cmdutiltest.New(t)
	root := &cobra.Command{Use: "shithub"}
	root.AddCommand(NewCmd(tf.Factory))
	return root, tf
}

func TestCompletionRequiresShellFlag(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{IO: tf.IOStreams}
	if err := Run(context.Background(), &cobra.Command{Use: "shithub"}, opts); err == nil {
		t.Fatal("expected error without --shell")
	}
}

func TestCompletionUnsupportedShell(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{IO: tf.IOStreams, Shell: "nushell"}
	if err := Run(context.Background(), &cobra.Command{Use: "shithub"}, opts); err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

func TestCompletionBashScriptEmitted(t *testing.T) {
	root, tf := buildRootWithCompletion(t)
	opts := &options{IO: tf.IOStreams, Shell: "bash"}
	if err := Run(context.Background(), root, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	// Cobra's bash completion always starts with a shebang-ish marker.
	if !strings.Contains(out, "# bash completion V2") && !strings.Contains(out, "__shithub") {
		t.Errorf("bash completion script missing markers, got first 200 bytes: %q", firstN(out, 200))
	}
}

func TestCompletionEachSupportedShell(t *testing.T) {
	for _, shell := range supportedShells {
		t.Run(shell, func(t *testing.T) {
			root, tf := buildRootWithCompletion(t)
			opts := &options{IO: tf.IOStreams, Shell: shell}
			if err := Run(context.Background(), root, opts); err != nil {
				t.Errorf("shell %q: %v", shell, err)
			}
			if tf.Out.Len() == 0 {
				t.Errorf("shell %q produced no output", shell)
			}
		})
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
