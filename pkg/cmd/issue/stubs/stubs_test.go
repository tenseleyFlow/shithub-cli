// SPDX-License-Identifier: AGPL-3.0-or-later

package stubs

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
)

// runForTest invokes the cobra RunE with a noop command so we can
// assert the friendly error without spinning up the whole CLI.
func runForTest(t *testing.T, c *cobra.Command, args []string) error {
	t.Helper()
	if c.RunE == nil {
		t.Fatal("command has no RunE")
	}
	return c.RunE(c, args)
}

func TestPinReturnsNotYet(t *testing.T) {
	err := runForTest(t, NewPinCmd(nil), []string{"1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !cmdutil.IsNotYetSupported(err) {
		t.Errorf("unexpected error type: %T (%v)", err, err)
	}
}

func TestUnpinReturnsNotYet(t *testing.T) {
	if err := runForTest(t, NewUnpinCmd(nil), []string{"1"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestTransferReturnsNotYet(t *testing.T) {
	if err := runForTest(t, NewTransferCmd(nil), []string{"1", "o/r"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestDevelopReturnsNotYet(t *testing.T) {
	if err := runForTest(t, NewDevelopCmd(nil), []string{"1"}); err == nil {
		t.Fatal("expected error")
	}
}

// TestPinAcceptsRepoFlag covers E21: pin/unpin/transfer used to reject
// `-R` because their stubs declared zero flags. The deferred helper
// now registers --repo/-R so users in a non-default cwd can still hit
// the friendly notice.
func TestPinAcceptsRepoFlag(t *testing.T) {
	cmd := NewPinCmd(nil)
	cmd.SetArgs([]string{"-R", "alice/demo", "1"})
	cmd.SetOut(nopWriter{})
	cmd.SetErr(nopWriter{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected NotYetSupportedError, got nil")
	}
	if !cmdutil.IsNotYetSupported(err) {
		t.Errorf("unexpected: %T %v", err, err)
	}
}

// TestPinAcceptsUnknownFlags covers E20: deferred stubs must still
// reach RunE when the user passes unknown flags (gh-style invocations).
func TestPinAcceptsUnknownFlags(t *testing.T) {
	cmd := NewPinCmd(nil)
	cmd.SetArgs([]string{"--bogus-flag", "v1", "1"})
	cmd.SetOut(nopWriter{})
	cmd.SetErr(nopWriter{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected NotYetSupportedError, got nil")
	}
	if !cmdutil.IsNotYetSupported(err) {
		t.Errorf("unexpected: %T %v", err, err)
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
