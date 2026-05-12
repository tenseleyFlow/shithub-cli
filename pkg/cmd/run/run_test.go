// SPDX-License-Identifier: AGPL-3.0-or-later

package run

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func captureExit(t *testing.T) (*int, func()) {
	t.Helper()
	prev := exitFn
	var got int
	exitFn = func(code int) { got = code }
	return &got, func() { exitFn = prev }
}

func findSubcommand(cmds []*cobra.Command, name string) *cobra.Command {
	for _, c := range cmds {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestDeferredStubsExit(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	for _, name := range []string{"rerun", "cancel", "delete"} {
		t.Run(name, func(t *testing.T) {
			sub := findSubcommand(parent.Commands(), name)
			if sub == nil {
				t.Fatalf("subcommand %q not registered", name)
			}
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
		})
	}
}

func TestParentRegistersExpectedSubcommands(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	want := map[string]bool{
		"list": true, "view": true, "watch": true, "download": true,
		"rerun": true, "cancel": true, "delete": true,
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
