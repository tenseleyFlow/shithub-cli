// SPDX-License-Identifier: AGPL-3.0-or-later

package ruleset

import (
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// TestEverySubcommandDefers walks every registered subcommand and
// asserts the deferred message contract: each subcommand returns a
// NotYetSupportedError that the root translates into exit 2.
func TestEverySubcommandDefers(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)

	for _, sub := range parent.Commands() {
		name := sub.Name()
		t.Run(name, func(t *testing.T) {
			err := sub.RunE(sub, nil)
			if err == nil {
				t.Fatal("RunE returned nil; expected NotYetSupportedError")
			}
			if !cmdutil.IsNotYetSupported(err) {
				t.Errorf("not a NotYetSupportedError: %v", err)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error should reference subcommand name %q: %v", name, err)
			}
		})
	}
}

// TestParentRegistersExpectedSubcommands guards the wired set.
func TestParentRegistersExpectedSubcommands(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	want := map[string]bool{"list": true, "view": true, "check": true, "history": true}
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

// TestSubcommandAcceptsUniversalFlags pins H22/H23: every deferred
// stub now accepts -R / --hostname / --json / --jq / --template at
// flag parsing (UnknownFlags whitelist) — the deferred notice fires
// regardless of how the user invoked the command.
func TestSubcommandAcceptsUniversalFlags(t *testing.T) {
	for _, args := range [][]string{
		{"list", "-R", "foo/bar"},
		{"list", "--hostname", "shithub.sh"},
		{"list", "--json", "name"},
		{"list", "--jq", ".name"},
		{"list", "--template", "{{.}}"},
	} {
		tf := cmdutiltest.New(t)
		parent := NewCmd(tf.Factory)
		parent.SetArgs(args)
		parent.SetErr(tf.ErrOut)
		parent.SetOut(tf.Out)
		err := parent.Execute()
		if err == nil {
			t.Errorf("args=%v: want NotYetSupportedError, got nil", args)
			continue
		}
		if !cmdutil.IsNotYetSupported(err) {
			t.Errorf("args=%v: want NotYetSupportedError, got %v", args, err)
		}
	}
}
