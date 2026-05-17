// SPDX-License-Identifier: AGPL-3.0-or-later

// Package iostreams centralizes all terminal I/O for shithub-cli: stdin,
// stdout, stderr, TTY detection, color flags, pager management, and the
// glamour-backed markdown renderer. Command code must never reach for
// os.Stdout / os.Stderr directly; it always goes through an *IOStreams
// instance passed via cobra's command context. This single seam lets tests
// capture output, lets the pipeline negotiate paging vs raw output, and
// makes color/TTY behavior deterministic.
package iostreams

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

// Env-var names recognized by iostreams. Centralized so callers can grep
// for SHITHUB_FORCE_TTY etc. and find the contract here.
const (
	EnvNoColor       = "NO_COLOR"          // any non-empty value disables color
	EnvCLIColor      = "CLICOLOR"          // "0" disables; otherwise default-by-TTY
	EnvCLIColorForce = "CLICOLOR_FORCE"    // non-"0" forces color even off-TTY
	EnvForceTTY      = "SHITHUB_FORCE_TTY" // pretend stdout is a TTY
	EnvTerm          = "TERM"
	EnvColorTerm     = "COLORTERM"
)

// IOStreams is the single I/O surface every command writes through.
// Instances are cheap to build; create one per command invocation rather
// than reusing globally.
type IOStreams struct {
	// In is the stdin reader. Closing it is a no-op against os.Stdin; tests
	// using bytes.NewReader / strings.NewReader get a no-op closer.
	In io.ReadCloser
	// Out is the primary output stream. When a pager is active, Out becomes
	// a write-end of a pipe feeding the pager process.
	Out io.Writer
	// ErrOut is the diagnostic stream. Never paged; never JSON-projected.
	ErrOut io.Writer

	stdoutTTY bool
	stderrTTY bool
	stdinTTY  bool

	colorEnabled  bool
	colorIs256    bool
	colorIsTrue24 bool

	// pager state — see pager.go.
	pager pagerState

	// neverPrompt is set in Test() so interactive prompt helpers fail fast
	// rather than hang on a closed stdin.
	neverPrompt bool
}

// System returns an IOStreams wired to the running process's stdin/stdout/
// stderr. TTY status and color flags are resolved from the environment at
// construction time; subsequent env changes do not retroactively flip them.
func System() *IOStreams {
	stdoutTTY := isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
	stderrTTY := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	stdinTTY := isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())

	if os.Getenv(EnvForceTTY) != "" {
		stdoutTTY = true
	}

	color, c256, ctrue := resolveColor(stdoutTTY)

	return &IOStreams{
		In:            os.Stdin,
		Out:           os.Stdout,
		ErrOut:        os.Stderr,
		stdoutTTY:     stdoutTTY,
		stderrTTY:     stderrTTY,
		stdinTTY:      stdinTTY,
		colorEnabled:  color,
		colorIs256:    c256,
		colorIsTrue24: ctrue,
	}
}

// Test returns an IOStreams backed by in-memory buffers, plus handles to
// the buffers so tests can read what was written. Color is disabled and
// no stream is reported as a TTY; prompts fail fast via neverPrompt.
//
// Returned buffers are *bytes.Buffer — callers can read with .String() or
// .Bytes() and inspect at any point during the test.
func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	streams := &IOStreams{
		In:        io.NopCloser(in),
		Out:       out,
		ErrOut:    errOut,
		stdoutTTY: false,
		stderrTTY: false,
		stdinTTY:  false,
		// Color stays off by default in tests so golden-file comparisons are
		// stable. Callers needing color can flip with SetColorEnabled.
		colorEnabled: false,
		neverPrompt:  true,
	}
	return streams, in, out, errOut
}

// IsStdoutTTY reports whether stdout is connected to a terminal.
// Honored by pager activation, spinner display, and color resolution.
func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutTTY }

// IsStderrTTY reports whether stderr is connected to a terminal.
func (s *IOStreams) IsStderrTTY() bool { return s.stderrTTY }

// IsStdinTTY reports whether stdin is connected to a terminal.
// Commands use this to decide whether to launch interactive prompts.
func (s *IOStreams) IsStdinTTY() bool { return s.stdinTTY }

// ColorEnabled reports whether ANSI color output is enabled.
// Respects NO_COLOR, CLICOLOR, CLICOLOR_FORCE, and TTY status per the
// de-facto Unix color convention (see https://no-color.org).
func (s *IOStreams) ColorEnabled() bool { return s.colorEnabled }

// Color256Enabled reports whether the terminal supports the 256-color
// palette. Implies ColorEnabled.
func (s *IOStreams) Color256Enabled() bool { return s.colorIs256 }

// ColorTrueEnabled reports whether the terminal supports 24-bit truecolor.
// Implies Color256Enabled and ColorEnabled.
func (s *IOStreams) ColorTrueEnabled() bool { return s.colorIsTrue24 }

// SetColorEnabled overrides the auto-detected color state. Primarily for
// tests; production code lets resolveColor decide.
func (s *IOStreams) SetColorEnabled(enabled bool) { s.colorEnabled = enabled }

// SetStdoutTTY overrides the detected TTY status of stdout. Used by tests
// that need to exercise TTY-only code paths against buffer-backed streams.
func (s *IOStreams) SetStdoutTTY(tty bool) { s.stdoutTTY = tty }

// NeverPrompt reports whether interactive prompts should refuse to run.
// True in Test() streams; commands check this before invoking survey/huh.
func (s *IOStreams) NeverPrompt() bool { return s.neverPrompt }

// SetNeverPrompt overrides the no-prompt flag — tests opt back into
// interactive paths by flipping this to false, after wiring the
// prompter fake with the desired responses.
func (s *IOStreams) SetNeverPrompt(v bool) { s.neverPrompt = v }

// TerminalWidth returns the detected terminal width in columns. Returns
// DefaultTerminalWidth when stdout is not a TTY or detection fails.
func (s *IOStreams) TerminalWidth() int {
	if !s.stdoutTTY {
		return DefaultTerminalWidth
	}
	f, ok := s.Out.(*os.File)
	if !ok {
		return DefaultTerminalWidth
	}
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil || w <= 0 {
		return DefaultTerminalWidth
	}
	return w
}

// DefaultTerminalWidth is the fallback column count for tableprinter and
// markdown rendering when stdout is not a TTY (pipe, file, test) and we
// must still pick a reasonable wrap width.
const DefaultTerminalWidth = 80

// resolveColor returns (enabled, is256, isTrueColor) from environment.
// Precedence:
//  1. NO_COLOR set (any non-empty value) → off.
//  2. CLICOLOR_FORCE set to non-"0" → on (overrides TTY).
//  3. CLICOLOR == "0" → off.
//  4. Otherwise, default to stdoutTTY.
//
// 256-color and truecolor are derived from TERM / COLORTERM heuristics.
func resolveColor(stdoutTTY bool) (enabled, is256, isTrue bool) {
	if os.Getenv(EnvNoColor) != "" {
		return false, false, false
	}

	if v := os.Getenv(EnvCLIColorForce); v != "" && v != "0" {
		enabled = true
	} else if v := os.Getenv(EnvCLIColor); v == "0" {
		enabled = false
	} else {
		enabled = stdoutTTY
	}

	if !enabled {
		return false, false, false
	}

	term := strings.ToLower(os.Getenv(EnvTerm))
	colorTerm := strings.ToLower(os.Getenv(EnvColorTerm))

	isTrue = colorTerm == "truecolor" || colorTerm == "24bit"
	is256 = isTrue || strings.Contains(term, "256")
	return enabled, is256, isTrue
}
