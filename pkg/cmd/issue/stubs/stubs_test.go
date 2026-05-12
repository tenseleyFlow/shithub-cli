// SPDX-License-Identifier: AGPL-3.0-or-later

package stubs

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
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
	if err := runForTest(t, NewPinCmd(nil), []string{"1"}); err == nil {
		t.Fatal("expected error")
	} else {
		var nys notYetSupportedError
		if !errors.As(err, &nys) {
			t.Errorf("unexpected error type: %T", err)
		}
		if nys.Name != "pin" {
			t.Errorf("Name: %q", nys.Name)
		}
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
