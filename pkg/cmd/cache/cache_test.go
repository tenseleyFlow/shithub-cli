// SPDX-License-Identifier: AGPL-3.0-or-later

package cache

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func findSubcommand(cmds []*cobra.Command, name string) *cobra.Command {
	for _, c := range cmds {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestDeferredStubExits(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)

	sub := findSubcommand(parent.Commands(), "delete")
	if sub == nil {
		t.Fatal("delete subcommand not registered")
	}
	err := sub.RunE(sub, nil)
	if !cmdutil.IsNotYetSupported(err) {
		t.Errorf("want NotYetSupportedError, got %v", err)
	}
	if !strings.Contains(err.Error(), "delete") {
		t.Errorf("error should reference subcommand: %v", err)
	}
}

func TestParentRegistersExpectedSubcommands(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	want := map[string]bool{"list": true, "delete": true}
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
