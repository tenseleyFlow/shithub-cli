// SPDX-License-Identifier: AGPL-3.0-or-later

package attestation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func captureExit(t *testing.T) (*int, func()) {
	t.Helper()
	prev := exitFn
	var got int
	exitFn = func(code int) { got = code }
	return &got, func() { exitFn = prev }
}

// TestEverySubcommandDefers walks every registered subcommand and
// asserts the deferred message + exit 2 contract.
func TestEverySubcommandDefers(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)

	for _, sub := range parent.Commands() {
		name := sub.Name()
		t.Run(name, func(t *testing.T) {
			gotCode, restore := captureExit(t)
			t.Cleanup(restore)

			var stderr bytes.Buffer
			sub.SetErr(&stderr)
			sub.SetOut(&bytes.Buffer{})

			if err := sub.RunE(sub, nil); err != nil {
				t.Fatalf("RunE: %v", err)
			}
			if *gotCode != deferredExitCode {
				t.Errorf("exit code: want %d got %d", deferredExitCode, *gotCode)
			}
			if !strings.Contains(stderr.String(), deferredMessage) {
				t.Errorf("stderr missing deferred message: %q", stderr.String())
			}
			if !strings.Contains(stderr.String(), name) {
				t.Errorf("stderr missing subcommand name %q: %q", name, stderr.String())
			}
		})
	}
}

// TestParentRegistersExpectedSubcommands guards the wired set.
func TestParentRegistersExpectedSubcommands(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	want := map[string]bool{
		"verify": true, "download": true, "inspect": true, "trusted-root": true,
	}
	got := map[string]bool{}
	for _, sub := range parent.Commands() {
		got[sub.Name()] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d subcommands, want %d (got=%v)", len(got), len(want), got)
	}
}
