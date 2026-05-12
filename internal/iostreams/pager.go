// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/cli/safeexec"
)

// Env-var names governing pager selection. Centralized for grep-ability.
const (
	EnvPager        = "SHITHUB_PAGER" // shithub-specific override
	EnvPagerGeneric = "PAGER"         // standard Unix env
)

// DefaultPagerUnix is the canonical fallback. The flags:
//
//	-F: exit if output fits on one screen (don't pager a one-liner)
//	-R: pass through ANSI color escapes (preserve our color output)
//	-X: don't clear the screen on exit (keep output visible after quitting)
//
// Matches gh's default; users familiar with `gh` get the same behavior.
const DefaultPagerUnix = "less -FRX"

// DefaultPagerWindows is the obvious fallback. Windows users typically
// override via the PAGER env var; `more` is the lowest-common-denominator.
const DefaultPagerWindows = "more"

// pagerState holds the live process and the original Out writer so
// StopPager can restore the stream cleanly. Zero value means "no pager
// active" — fields are populated only between StartPager and StopPager.
type pagerState struct {
	cmd      *exec.Cmd
	pipe     io.WriteCloser
	original io.Writer // s.Out before we swapped in the pipe
	disabled bool      // explicit --no-pager equivalent
}

// SetPagerDisabled blocks future StartPager calls from spawning a process.
// Used by the --no-pager root flag and by the JSON-output path (machine-
// readable output is never paged).
func (s *IOStreams) SetPagerDisabled(disabled bool) {
	s.pager.disabled = disabled
}

// PagerDisabled reports whether the pager has been explicitly disabled.
func (s *IOStreams) PagerDisabled() bool { return s.pager.disabled }

// ResolvePager returns the pager command line as a slice of [program, args...].
// Precedence:
//  1. configPager (the user's `shithub config get pager` value).
//  2. SHITHUB_PAGER env var.
//  3. PAGER env var.
//  4. Platform default (less -FRX on Unix, more on Windows).
//
// Returns (nil, false) when paging should be skipped — either the pager
// is explicitly empty (a deliberate "disable" signal) or stdout is not a
// terminal. Callers should also short-circuit on PagerDisabled().
func (s *IOStreams) ResolvePager(configPager string) ([]string, bool) {
	if !s.stdoutTTY {
		return nil, false
	}
	if s.pager.disabled {
		return nil, false
	}

	// Empty config-set pager is a deliberate disable signal.
	if configPager != "" {
		return splitPager(configPager), true
	}
	if v, ok := os.LookupEnv(EnvPager); ok {
		if v == "" {
			return nil, false
		}
		return splitPager(v), true
	}
	if v, ok := os.LookupEnv(EnvPagerGeneric); ok {
		if v == "" {
			return nil, false
		}
		return splitPager(v), true
	}

	if runtime.GOOS == "windows" {
		return splitPager(DefaultPagerWindows), true
	}
	return splitPager(DefaultPagerUnix), true
}

// StartPager spawns the resolved pager and rewires Out to feed its stdin.
// configPager is typically the value from internal/config; pass "" if the
// caller doesn't have one. After StartPager returns nil, every Out write
// is paged. Call StopPager (typically via defer) to close the pipe and
// wait on the child.
//
// Errors during spawn surface immediately. Pipe writes that fail because
// the user quit the pager early are NOT surfaced — they're translated to
// "no more output" via a discarding writer, so commands don't error out
// just because the user pressed `q`.
func (s *IOStreams) StartPager(configPager string) error {
	if s.pager.cmd != nil {
		return errors.New("pager: already active")
	}
	argv, ok := s.ResolvePager(configPager)
	if !ok {
		return nil // paging skipped; not an error
	}
	if len(argv) == 0 {
		return errors.New("pager: empty command")
	}

	bin, err := safeexec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("pager: locate %q: %w", argv[0], err)
	}

	cmd := exec.Command(bin, argv[1:]...) //nolint:gosec // argv is from trusted config/env
	cmd.Stdout = s.Out
	cmd.Stderr = s.ErrOut

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("pager: stdin pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("pager: start: %w", err)
	}

	s.pager.cmd = cmd
	s.pager.pipe = stdin
	s.pager.original = s.Out
	// brokenPipeWriter swallows EPIPE so writes after the user quits the
	// pager don't propagate as command-level errors.
	s.Out = &brokenPipeWriter{w: stdin}
	return nil
}

// StopPager closes the write end of the pager pipe, waits for the child
// to exit, and restores s.Out. Safe to call when no pager is active
// (no-op). Always pair StartPager with a deferred StopPager.
func (s *IOStreams) StopPager() {
	if s.pager.cmd == nil {
		return
	}
	if s.pager.pipe != nil {
		_ = s.pager.pipe.Close()
	}
	// We deliberately ignore Wait errors. The pager exiting non-zero (e.g.,
	// `less` returning 2 on a missing terminfo entry) does not invalidate
	// command output; surfacing it would be noise.
	_ = s.pager.cmd.Wait()
	if s.pager.original != nil {
		s.Out = s.pager.original
	}
	s.pager.cmd = nil
	s.pager.pipe = nil
	s.pager.original = nil
}

// splitPager tokenizes a pager command line on whitespace. This is the
// pragmatic-not-perfect choice: it does not handle quoted args with spaces
// (e.g. PAGER='less -R "with quoted thing"'). gh has the same limit; we
// document it instead of pulling in a full shell-quote parser.
func splitPager(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

// brokenPipeWriter wraps an io.WriteCloser and translates EPIPE into a
// silent short-write. Necessary because pagers like `less` close their
// stdin when the user quits, and any subsequent write would otherwise
// bubble back as "broken pipe" — which is correct semantically but
// useless to the user.
type brokenPipeWriter struct {
	w io.Writer
}

func (b *brokenPipeWriter) Write(p []byte) (int, error) {
	n, err := b.w.Write(p)
	if err != nil && isBrokenPipe(err) {
		return len(p), nil
	}
	return n, err
}

// isBrokenPipe inspects an error chain for the OS-level EPIPE signal.
// We can't compare to a sentinel because syscall.EPIPE isn't returned
// uniformly across platforms; substring match on "broken pipe" is the
// reliable cross-platform check the Go ecosystem already uses.
func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "broken pipe") || strings.Contains(msg, "EPIPE")
}
