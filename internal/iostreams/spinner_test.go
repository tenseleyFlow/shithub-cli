// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import "testing"

func TestSpinnerNoOpOnNonTTY(t *testing.T) {
	t.Parallel()
	s, _, _, errOut := Test()
	// Test() defaults to stdoutTTY=false; spinner must not draw anything.

	stop := s.StartSpinner("looking up...")
	stop()

	if errOut.Len() != 0 {
		t.Errorf("spinner on non-TTY should produce no output; got %q", errOut.String())
	}
}

func TestSpinnerStopSafeWithoutStart(t *testing.T) {
	t.Parallel()
	// StartSpinner always returns a usable stop func; calling it after a
	// no-op start must not panic or write anything weird.
	s, _, _, _ := Test()
	stop := s.StartSpinner("anything")
	stop()
	stop() // second invocation also fine (defensive)
}
