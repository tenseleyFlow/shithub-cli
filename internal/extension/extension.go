// SPDX-License-Identifier: AGPL-3.0-or-later

// Package extension owns the on-disk layout, discovery, exec, and
// scaffolding for third-party `shithub-<verb>` extensions. The
// dispatcher in cmd/shithub/root.go calls Find + Exec; the
// pkg/cmd/extension surface composes List / Install / Upgrade / Remove
// / Create on top of these primitives.
//
// On-disk shape:
//
//	${SHITHUB_CONFIG_DIR}/extensions/
//	  shithub-<verb>/
//	    shithub-<verb>          # the executable (script or compiled binary)
//	    .git/                    # source-installed extensions are git repos
//
// The executable must be named exactly `shithub-<verb>` with the
// executable bit set; gh's convention matches and we follow it.
package extension

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Prefix is the directory + executable prefix that marks something as
// a shithub extension. Repos named `shithub-<verb>` install to a
// `shithub-<verb>/` subdir of the extensions root.
const Prefix = "shithub-"

// Installed describes one resolved extension. Path is the directory
// (not the executable inside it).
type Installed struct {
	Name string // verb, with no `shithub-` prefix
	Path string // <extensions-dir>/shithub-<name>
}

// List enumerates the installed extensions under root. Returns an empty
// slice when root doesn't exist yet — that's the common first-run case.
func List(root string) ([]Installed, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("extension: read %s: %w", root, err)
	}
	out := make([]Installed, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, Prefix) {
			continue
		}
		verb := strings.TrimPrefix(name, Prefix)
		if verb == "" {
			continue
		}
		out = append(out, Installed{Name: verb, Path: filepath.Join(root, name)})
	}
	return out, nil
}

// Find resolves a verb to an installed extension's executable path, or
// returns ("", false) if no extension provides that verb. Honors the
// platform-specific executable naming convention (`.exe` on Windows).
func Find(root, verb string) (string, bool) {
	if verb == "" || strings.ContainsAny(verb, "/\\") {
		return "", false
	}
	dir := filepath.Join(root, Prefix+verb)
	exe := filepath.Join(dir, Prefix+verb)
	if runtime.GOOS == "windows" {
		// Try both `shithub-verb` (gh-style scripts) and
		// `shithub-verb.exe` (compiled binaries) — gh allows both.
		if isExec(exe + ".exe") {
			return exe + ".exe", true
		}
	}
	if isExec(exe) {
		return exe, true
	}
	return "", false
}

// isExec reports whether path exists and is executable. On Windows the
// executable bit isn't a thing — existence + non-directory is enough;
// the .exe extension check is the caller's job.
func isExec(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// Exec runs the extension binary, forwarding args as argv[1:] and
// passing through the parent process's stdin/stdout/stderr. Returns
// the extension's exit code; for non-Exit errors (failed spawn, etc.)
// returns 1 and writes the error to stderr.
//
// Env vars SHITHUB_CONFIG_DIR / SHITHUB_HOST / SHITHUB_TOKEN are
// expected to be set by the caller before invoking; we pass through
// the existing os.Environ() unchanged.
func Exec(path string, args []string) int {
	cmd := exec.Command(path, args...) //nolint:gosec // user-installed extension
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "shithub: extension:", err)
		return 1
	}
	return 0
}

// Scaffold creates a starter `shithub-<name>/` extension repo at root.
// Today we ship the simplest possible scaffolding: an executable
// `shithub-<name>` POSIX script plus a README.  Compiled-Go and other
// templates are deferred until there's actual demand.  Existing
// directories are rejected unless force=true.
func Scaffold(root, name string, force bool) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir := filepath.Join(root, Prefix+name)
	if _, err := os.Stat(dir); err == nil {
		if !force {
			return "", fmt.Errorf("extension: %s already exists (use --force to overwrite)", dir)
		}
		if err := os.RemoveAll(dir); err != nil {
			return "", fmt.Errorf("extension: remove existing: %w", err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // user-owned scaffold
		return "", fmt.Errorf("extension: mkdir: %w", err)
	}
	script := fmt.Sprintf(`#!/usr/bin/env bash
# shithub-%s — a shithub extension. Invoked as `+"`shithub %s [args...]`"+`.
#
# Environment provided by the host:
#   SHITHUB_CONFIG_DIR  — config dir, contains hosts.yml etc.
#   SHITHUB_HOST        — default host
#   SHITHUB_TOKEN       — auth token (treat as sensitive)
#
# Prefer calling `+"`shithub api`"+` over direct curl so auth + retries
# stay consistent with the rest of the CLI.

set -euo pipefail
echo "hello from shithub-%s"
echo "args: $*"
`, name, name, name)
	scriptPath := filepath.Join(dir, Prefix+name)
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil { //nolint:gosec // intentional +x
		return "", fmt.Errorf("extension: write script: %w", err)
	}
	readme := fmt.Sprintf("# shithub-%s\n\nA shithub-cli extension. Install with:\n\n```\nshithub extension install <owner>/shithub-%s\n```\n", name, name)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil { //nolint:gosec // user-owned scaffold
		return "", fmt.Errorf("extension: write README: %w", err)
	}
	return dir, nil
}

// ValidateName rejects names that would let an attacker scribble outside
// the extensions dir or shadow built-in commands. Allowed: kebab-case
// ASCII, must start with a letter.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("extension: name is required")
	}
	if strings.ContainsAny(name, "/\\.. ") {
		return fmt.Errorf("extension: invalid name %q (no path separators, dots, or spaces)", name)
	}
	first := name[0]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') {
		return fmt.Errorf("extension: invalid name %q (must start with a letter)", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("extension: invalid character %q in name %q (allowed: a-z, A-Z, 0-9, -, _)", r, name)
		}
	}
	return nil
}

// VerbFromRepo derives an extension verb from an owner/repo slug. The
// repo MUST begin with `shithub-`; the returned verb is the part after
// that prefix. Returns "" if the repo doesn't follow the convention.
func VerbFromRepo(ownerRepo string) (string, bool) {
	_, repo, ok := strings.Cut(ownerRepo, "/")
	if !ok || repo == "" {
		return "", false
	}
	if !strings.HasPrefix(repo, Prefix) {
		return "", false
	}
	verb := strings.TrimPrefix(repo, Prefix)
	if err := ValidateName(verb); err != nil {
		return "", false
	}
	return verb, true
}
