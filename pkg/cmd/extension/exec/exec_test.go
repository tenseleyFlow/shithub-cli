// SPDX-License-Identifier: AGPL-3.0-or-later

package exec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestExecRunsAndForwardsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-exec on Windows needs a different harness")
	}
	tf := cmdutiltest.New(t)
	dir := t.TempDir()
	extDir := filepath.Join(dir, "shithub-foo")
	_ = os.MkdirAll(extDir, 0o755)
	exe := filepath.Join(extDir, "shithub-foo")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit $#\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var got int
	opts := &options{
		IO: tf.IOStreams, Name: "foo", Args: []string{"a", "b"},
		Dir: dir, exitFn: func(c int) { got = c },
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != 2 {
		t.Errorf("exit forwarding: want 2 (argc), got %d", got)
	}
}

func TestExecRejectsUninstalled(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{IO: tf.IOStreams, Name: "nope", Dir: t.TempDir(), exitFn: func(int) {}}
	if err := Run(context.Background(), opts); err == nil {
		t.Error("expected error for missing extension")
	}
}
