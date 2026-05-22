// SPDX-License-Identifier: AGPL-3.0-or-later

package run

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

func TestDeferredStubsExit(t *testing.T) {
	tf := cmdutiltest.New(t)
	parent := NewCmd(tf.Factory)
	for _, name := range []string{"rerun", "cancel", "delete"} {
		t.Run(name, func(t *testing.T) {
			sub := findSubcommand(parent.Commands(), name)
			if sub == nil {
				t.Fatalf("subcommand %q not registered", name)
			}
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
