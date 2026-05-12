// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !windows

package config

import "os"

// stat is a thin wrapper so the Unix-only perm assertion test in paths_test.go
// can reach os.Stat without pulling in Windows-specific build tags.
func stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
