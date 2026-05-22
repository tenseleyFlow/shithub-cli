// SPDX-License-Identifier: AGPL-3.0-or-later

package gist

import (
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// TestEverySubcommandDefers walks each registered gist subcommand and
// asserts that invoking it returns a NotYetSupportedError.
func TestEverySubcommandDefers(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)

	for _, sub := range parent.Commands() {
		name := sub.Name()
		t.Run(name, func(t *testing.T) {
			err := sub.RunE(sub, nil)
			if !cmdutil.IsNotYetSupported(err) {
				t.Errorf("want NotYetSupportedError, got %v", err)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error should reference subcommand %q: %v", name, err)
			}
		})
	}
}

// TestParentRegistersExpectedSubcommands guards the wired set.
func TestParentRegistersExpectedSubcommands(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	want := map[string]bool{
		"create": true, "list": true, "view": true,
		"edit": true, "clone": true, "delete": true, "rename": true,
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
