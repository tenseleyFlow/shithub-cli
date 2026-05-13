// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTryExtensionEmpty makes sure empty/flag-only argv flows through
// to cobra rather than being claimed by the dispatcher.
func TestTryExtensionEmpty(t *testing.T) {
	for _, args := range [][]string{nil, {}, {"-h"}, {"--help"}} {
		if code, handled := tryExtension(args); handled {
			t.Errorf("tryExtension(%v): should not handle, got (%d, true)", args, code)
		}
	}
}

// TestTryExtensionBuiltinWins guards against an extension shadowing a
// real subcommand. We arrange a fake extensions dir holding a
// `shithub-issue` script, then verify the dispatcher refuses to call
// it because `issue` is a built-in cobra subcommand.
func TestTryExtensionBuiltinWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-exec on Windows needs a different harness")
	}
	dir := t.TempDir()
	t.Setenv("SHITHUB_CONFIG_DIR", dir)
	extRoot := filepath.Join(dir, "extensions", "shithub-issue")
	if err := os.MkdirAll(extRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(extRoot, "shithub-issue")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 99\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// `issue` is a real subcommand on rootCmd — the dispatcher must
	// return (0, false) so cobra handles it normally.
	code, handled := tryExtension([]string{"issue", "list"})
	if handled {
		t.Errorf("tryExtension(issue): claimed shadowed built-in, got (%d, true)", code)
	}
}

// TestTryExtensionFindsExtension positively confirms an unknown verb
// matching an installed extension is dispatched and its exit code is
// surfaced.
func TestTryExtensionFindsExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-exec on Windows needs a different harness")
	}
	dir := t.TempDir()
	t.Setenv("SHITHUB_CONFIG_DIR", dir)
	extRoot := filepath.Join(dir, "extensions", "shithub-hello")
	if err := os.MkdirAll(extRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(extRoot, "shithub-hello")
	// Exit with the count of args so we can verify pass-through.
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit $#\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, handled := tryExtension([]string{"hello", "a", "b"})
	if !handled {
		t.Fatal("tryExtension(hello a b): should be claimed")
	}
	if code != 2 {
		t.Errorf("exit code: want 2 (argc), got %d", code)
	}
}

// TestTryExtensionMiss confirms an unknown verb with no matching
// extension flows through to cobra (so cobra emits its usual error).
func TestTryExtensionMiss(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHITHUB_CONFIG_DIR", dir)
	if code, handled := tryExtension([]string{"definitely-not-a-thing"}); handled {
		t.Errorf("tryExtension(miss): should not handle, got (%d, true)", code)
	}
}
