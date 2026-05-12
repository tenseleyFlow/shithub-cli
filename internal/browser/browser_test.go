// SPDX-License-Identifier: AGPL-3.0-or-later

package browser

import (
	"errors"
	"testing"
)

func TestOverrideRoundTrip(t *testing.T) {
	var got string
	restore := Override(func(url string) error { got = url; return nil })
	defer restore()

	if err := Open("https://example.test/x"); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got != "https://example.test/x" {
		t.Errorf("Open passed wrong URL: %q", got)
	}
}

func TestOverrideRestores(t *testing.T) {
	called := false
	restore := Override(func(_ string) error { called = true; return nil })
	restore()

	// After restore the override should be gone — opener is nil.
	if opener != nil {
		t.Error("opener should be nil after restore")
	}
	if called {
		// The Open call below shouldn't even reach the override.
		t.Error("override was called after restore")
	}
}

func TestOverridePropagatesError(t *testing.T) {
	restore := Override(func(_ string) error { return errors.New("no") })
	defer restore()

	if err := Open("x"); err == nil {
		t.Fatal("expected error from override")
	}
}
