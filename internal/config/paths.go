// SPDX-License-Identifier: AGPL-3.0-or-later

// Package config owns shithub-cli's on-disk configuration substrate:
// the user-level preferences in config.yml and the per-host credentials
// in hosts.yml. Tokens prefer the system keyring; hosts.yml is the
// 0600 fallback. No command should read or write these files directly;
// go through this package.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Env-var names. Centralized so the rest of the CLI imports symbols, not
// strings. SHITHUB_CONFIG_DIR overrides everything below.
const (
	EnvConfigDir       = "SHITHUB_CONFIG_DIR"
	EnvXDGConfigHome   = "XDG_CONFIG_HOME"
	EnvHome            = "HOME"
	EnvWindowsAppData  = "AppData"
	EnvWindowsUserProf = "UserProfile"
)

// ConfigDir returns the absolute path of the shithub-cli config directory.
//
// Precedence:
//  1. $SHITHUB_CONFIG_DIR if set.
//  2. $XDG_CONFIG_HOME/shithub if set.
//  3. $HOME/.config/shithub on Unix (Linux, macOS, BSDs).
//  4. %AppData%\shithub on Windows.
//
// We deliberately do NOT use os.UserConfigDir on macOS because that
// returns ~/Library/Application Support, which diverges from gh's
// XDG-on-all-Unix convention; users moving between gh and shithub
// expect the same layout shape.
func ConfigDir() (string, error) {
	if dir := os.Getenv(EnvConfigDir); dir != "" {
		return dir, nil
	}
	if dir := os.Getenv(EnvXDGConfigHome); dir != "" {
		return filepath.Join(dir, "shithub"), nil
	}

	if runtime.GOOS == "windows" {
		if dir := os.Getenv(EnvWindowsAppData); dir != "" {
			return filepath.Join(dir, "shithub"), nil
		}
		if dir := os.Getenv(EnvWindowsUserProf); dir != "" {
			return filepath.Join(dir, "AppData", "Roaming", "shithub"), nil
		}
		return "", errors.New("config: cannot resolve config dir (no AppData or UserProfile)")
	}

	home := os.Getenv(EnvHome)
	if home == "" {
		return "", errors.New("config: cannot resolve config dir (HOME unset)")
	}
	return filepath.Join(home, ".config", "shithub"), nil
}

// ConfigFile returns the path of the user-level config.yml.
func ConfigFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yml"), nil
}

// HostsFile returns the path of the per-host hosts.yml. This file holds
// tokens when the system keyring is unavailable (or --insecure-storage
// is requested); its on-disk permissions must be 0600.
func HostsFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hosts.yml"), nil
}

// CacheDir returns the directory used for HTTP response caching
// (shithub api --cache) and any other ephemeral state.
func CacheDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache"), nil
}

// ExtensionsDir returns the directory where third-party `shithub-<verb>`
// extensions are installed. The dispatcher in cmd/shithub/root.go looks
// here when a top-level verb doesn't match a built-in.
func ExtensionsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "extensions"), nil
}

// EnsureDir creates the config directory tree with 0700 permissions if it
// does not already exist. It is safe to call repeatedly. On existing
// directories we do not chmod — a user who has loosened perms intentionally
// gets to keep them; the 0600 hosts.yml check enforces the security boundary.
func EnsureDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("config: mkdir %s: %w", dir, err)
	}
	return dir, nil
}
