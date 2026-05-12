// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "os"

// readFile is split out so api.go doesn't import os directly; the
// variable indirection (readFileContents) keeps tests able to swap if
// they ever need to.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec // user-supplied path is the whole point of --input
}
