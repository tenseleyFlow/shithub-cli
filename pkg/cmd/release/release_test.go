// SPDX-License-Identifier: AGPL-3.0-or-later

package release

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// captureExit swaps exitFn for a recorder that stores the code instead
// of terminating the test process. Returns the captured code pointer
// and a restore func.
func captureExit(t *testing.T) (*int, func()) {
	t.Helper()
	prev := exitFn
	var got int
	exitFn = func(code int) { got = code }
	return &got, func() { exitFn = prev }
}

// TestEverySubcommandDefers walks each registered release subcommand
// and asserts that invoking it produces the deferred message + exit
// code 2. The walk catches new additions automatically — adding an
// unstubbed subcommand without the deferred Run would fail here.
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

// TestParentLists confirms the parent registers exactly the eight
// subcommands the spec enumerates — guards against an accidental
// drop / rename when the real implementation lands.
func TestParentListsExpectedSubcommands(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	want := map[string]bool{
		"create": true, "list": true, "view": true,
		"upload": true, "download": true, "delete": true,
		"delete-asset": true, "edit": true,
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
