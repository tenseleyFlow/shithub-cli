// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// withConfigDir points the config package at a fresh-but-uncreated path
// inside t.TempDir(), so EnsureDir actually creates it with the contracted
// 0700 perms instead of inheriting t.TempDir()'s 0755 default.
func withConfigDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shithub")
	t.Setenv(EnvConfigDir, dir)
	return dir
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	withConfigDir(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load on empty dir: %v", err)
	}
	if c.Version != SchemaVersion {
		t.Errorf("Version: want %d got %d", SchemaVersion, c.Version)
	}
	if c.GitProtocol != DefaultGitProtocol {
		t.Errorf("GitProtocol: want %q got %q", DefaultGitProtocol, c.GitProtocol)
	}
	if c.Prompt != DefaultPrompt {
		t.Errorf("Prompt: want %q got %q", DefaultPrompt, c.Prompt)
	}
	if c.Aliases == nil {
		t.Error("Aliases: want non-nil empty map, got nil")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withConfigDir(t)

	src := Default()
	src.Editor = "code -w"
	src.Browser = "firefox"
	src.Pager = "less -FRX"
	src.GitProtocol = GitProtocolSSH
	src.Aliases["co"] = "pr checkout"
	src.Aliases["mine"] = "pr list --author @me"

	if err := src.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Editor != src.Editor {
		t.Errorf("Editor: want %q got %q", src.Editor, got.Editor)
	}
	if got.Browser != src.Browser {
		t.Errorf("Browser: want %q got %q", src.Browser, got.Browser)
	}
	if got.GitProtocol != src.GitProtocol {
		t.Errorf("GitProtocol: want %q got %q", src.GitProtocol, got.GitProtocol)
	}
	if got.Aliases["co"] != "pr checkout" {
		t.Errorf("Aliases[co]: want 'pr checkout' got %q", got.Aliases["co"])
	}
}

func TestSaveAtomicAndPermissive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits don't translate on Windows")
	}
	dir := withConfigDir(t)

	if err := Default().Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("config.yml perms: want 0644 got %o", perm)
	}

	// Parent dir should be 0700 (EnsureDir contract).
	dinfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if perm := dinfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("config dir perms: want 0700 got %o", perm)
	}
}

func TestSaveRejectsInvalidGitProtocol(t *testing.T) {
	withConfigDir(t)

	c := Default()
	c.GitProtocol = "ftp"
	if err := c.Save(); err == nil {
		t.Fatal("expected error on invalid GitProtocol")
	} else if !strings.Contains(err.Error(), "git_protocol") {
		t.Errorf("error should name the offending key, got: %v", err)
	}
}

func TestSaveRejectsInvalidPrompt(t *testing.T) {
	withConfigDir(t)

	c := Default()
	c.Prompt = "maybe"
	if err := c.Save(); err == nil {
		t.Fatal("expected error on invalid Prompt")
	}
}

func TestLoadMalformedYAMLReturnsError(t *testing.T) {
	dir := withConfigDir(t)
	if _, err := EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	junk := []byte("version: not-a-number: also-not\n :\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), junk, 0o644); err != nil {
		t.Fatalf("seed bad file: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// TestLoadBackfillsDefaults verifies that a file with only some keys set
// still produces a usable Config: missing keys take Default values.
func TestLoadBackfillsDefaults(t *testing.T) {
	dir := withConfigDir(t)
	if _, err := EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	partial := []byte("editor: emacs\n")
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), partial, 0o644); err != nil {
		t.Fatalf("seed partial file: %v", err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Editor != "emacs" {
		t.Errorf("Editor: want emacs got %q", c.Editor)
	}
	if c.GitProtocol != DefaultGitProtocol {
		t.Errorf("GitProtocol fallback: want %q got %q", DefaultGitProtocol, c.GitProtocol)
	}
	if c.Prompt != DefaultPrompt {
		t.Errorf("Prompt fallback: want %q got %q", DefaultPrompt, c.Prompt)
	}
}

// TestAtomicWriteSurvivesConcurrent verifies that two Saves running back to
// back leave a single well-formed file. We do not test true concurrency
// here (that's racy by design); we exercise the temp-file + rename path.
func TestAtomicWriteSurvivesConcurrent(t *testing.T) {
	dir := withConfigDir(t)

	for i := 0; i < 10; i++ {
		c := Default()
		c.Editor = "v" + string(rune('0'+i))
		if err := c.Save(); err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
	}

	// No leftover .tmp files in the config dir.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
