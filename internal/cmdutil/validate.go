// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil

import "fmt"

// ValidateLimit rejects zero and negative `--limit` values with the
// gh-compatible "invalid limit: N" error. List commands wire it before
// issuing the request so users get a clear refusal instead of the
// nonsensical-but-accepted behavior the C-audit C9 finding flagged
// (e.g. `--limit 0` silently returning a full page).
func ValidateLimit(n int) error {
	if n < 1 {
		return fmt.Errorf("invalid limit: %d", n)
	}
	return nil
}
