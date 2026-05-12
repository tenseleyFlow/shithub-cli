// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
)

// mkSeed populates a directory with a single placeholder file so the
// clear-cache test can assert "directory becomes empty afterward".
func mkSeed(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "seed"), []byte("x"), 0o600)
}

// readDir wraps os.ReadDir so the test file's import list stays focused
// on testify-style assertions rather than os boilerplate.
func readDir(dir string) ([]os.DirEntry, error) {
	return os.ReadDir(dir)
}
