// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// SchemaVersion is the current on-disk config.yml schema version.
// Migrations from older versions live in migrate.go. Bumping this is a
// breaking-change signal: existing files must round-trip through Migrate.
const SchemaVersion = 1

// Default values applied during Load when the file omits a key.
const (
	DefaultGitProtocol = "https"
	DefaultPrompt      = "enabled"
)

// Valid values for `prompt`. Validated on Save.
const (
	PromptEnabled  = "enabled"
	PromptDisabled = "disabled"
)

// Valid values for `git_protocol`.
const (
	GitProtocolHTTPS = "https"
	GitProtocolSSH   = "ssh"
)

// Config is the user-level preferences file (~/.config/shithub/config.yml).
// Aliases live here too; they're managed via the shithub alias subcommand
// (C06). Per-host overrides live in HostEntry, not here.
type Config struct {
	Version      int               `yaml:"version"`
	GitProtocol  string            `yaml:"git_protocol,omitempty"`
	Editor       string            `yaml:"editor,omitempty"`
	Browser      string            `yaml:"browser,omitempty"`
	Pager        string            `yaml:"pager,omitempty"`
	Prompt       string            `yaml:"prompt,omitempty"`
	HTTPUnixSock string            `yaml:"http_unix_socket,omitempty"`
	Aliases      map[string]string `yaml:"aliases,omitempty"`
}

// Default returns a freshly-initialized Config with documented defaults.
// Used both as a starting point for new installs and as the merge base
// when a file omits keys.
func Default() *Config {
	return &Config{
		Version:     SchemaVersion,
		GitProtocol: DefaultGitProtocol,
		Prompt:      DefaultPrompt,
		Aliases:     map[string]string{},
	}
}

// Load reads and parses config.yml from the resolved config directory.
// A missing file is not an error: Load returns the Default config and a
// nil error, so first-run commands behave correctly.
//
// On a malformed file Load returns an error with the file path so the
// user can fix it; we never silently rewrite a damaged config.
func Load() (*Config, error) {
	path, err := ConfigFile()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path) //nolint:gosec // path is from our own ConfigFile()
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	c := Default()
	if err := yaml.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	// Backfill defaults for keys the file omitted entirely (yaml.Unmarshal
	// leaves zero values rather than re-applying the Default() seeds).
	if c.GitProtocol == "" {
		c.GitProtocol = DefaultGitProtocol
	}
	if c.Prompt == "" {
		c.Prompt = DefaultPrompt
	}
	if c.Aliases == nil {
		c.Aliases = map[string]string{}
	}
	// Schema version is unconditionally re-stamped so old files migrate forward.
	c.Version = SchemaVersion

	return c, nil
}

// Save writes the Config to config.yml using an atomic temp-file rename so
// concurrent invocations cannot leave a half-written file. Permissions are
// 0644 on the file and 0700 on the parent directory; tokens live in
// hosts.yml (0600), not here.
func (c *Config) Save() error {
	if err := c.validate(); err != nil {
		return err
	}

	dir, err := EnsureDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yml")

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	return atomicWriteFile(path, data, 0o644)
}

// validate enforces the value constraints we surface to users via
// `shithub config set`. Loading a hand-edited file with bad values does
// NOT fail on Load — it fails on the next Save — so users can repair via
// config set without bouncing through Load errors.
func (c *Config) validate() error {
	switch c.GitProtocol {
	case "", GitProtocolHTTPS, GitProtocolSSH:
	default:
		return fmt.Errorf("config: git_protocol must be %q or %q, got %q",
			GitProtocolHTTPS, GitProtocolSSH, c.GitProtocol)
	}
	switch c.Prompt {
	case "", PromptEnabled, PromptDisabled:
	default:
		return fmt.Errorf("config: prompt must be %q or %q, got %q",
			PromptEnabled, PromptDisabled, c.Prompt)
	}
	return nil
}

// atomicWriteFile writes data to path via a same-directory temp file plus
// os.Rename so a crash mid-write never leaves a partially-written target.
// Callers pass the final permission mode; we Chmod the temp before rename
// so the visible file always has correct perms.
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp: %w", err)
	}
	tmpName := tmp.Name()

	// On any error after CreateTemp, scrub the temp so we don't litter the
	// dir with .*.tmp files. Rename below moves it; if rename succeeds, the
	// Remove call below is a no-op (the path no longer exists).
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("config: write %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("config: chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("config: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("config: rename %s -> %s: %w", tmpName, path, err)
	}
	return nil
}
