// SPDX-License-Identifier: AGPL-3.0-or-later

package release

import (
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// TestEverySubcommandDefers walks each registered release subcommand
// and asserts that invoking it returns a NotYetSupportedError. The
// walk catches new additions automatically — adding an unstubbed
// subcommand without the deferred Run would fail here.
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

// TestUniversalFlagsAccepted pins H22/H23: every deferred stub now
// accepts -R / --hostname / --json / --jq / --template at parse time.
func TestUniversalFlagsAccepted(t *testing.T) {
	for _, args := range [][]string{
		{"list", "-R", "foo/bar"},
		{"list", "--hostname", "shithub.sh"},
		{"list", "--json", "title"},
		{"list", "--jq", ".title"},
		{"list", "--template", "{{.}}"},
	} {
		tf := cmdutiltest.New(t)
		parent := NewCmd(tf.Factory)
		parent.SetArgs(args)
		parent.SetErr(tf.ErrOut)
		parent.SetOut(tf.Out)
		err := parent.Execute()
		if !cmdutil.IsNotYetSupported(err) {
			t.Errorf("args=%v: want NotYetSupportedError, got %v", args, err)
		}
	}
}
