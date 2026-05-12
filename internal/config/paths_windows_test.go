// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build windows

package config

import "os"

// stat exists as a placeholder so the cross-platform test in paths_test.go
// compiles on Windows. The 0700-permission test that consumes it skips
// itself on Windows; this implementation is never reached at runtime.
func stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
