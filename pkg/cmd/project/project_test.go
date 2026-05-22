// SPDX-License-Identifier: AGPL-3.0-or-later

package project

import (
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// TestEverySubcommandDefers walks every registered subcommand and
// asserts the NotYetSupportedError contract. Catches new stubs
// automatically.
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

// TestParentRegistersExpectedSubcommands guards the wired set against
// drift: 9 direct + 6 item-* + 3 field-* + 1 template-* = 19 stubs.
func TestParentRegistersExpectedSubcommands(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	want := map[string]bool{
		"list": true, "view": true, "create": true, "edit": true,
		"close": true, "copy": true, "delete": true, "link": true, "unlink": true,
		"item-list": true, "item-add": true, "item-create": true,
		"item-edit": true, "item-archive": true, "item-delete": true,
		"field-list": true, "field-create": true, "field-delete": true,
		"template-list": true,
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
