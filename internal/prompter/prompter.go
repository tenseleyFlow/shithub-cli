// SPDX-License-Identifier: AGPL-3.0-or-later

// Package prompter declares the interactive-prompt contract used by every
// command that needs to ask the user a question. The interface is defined
// here so tests can inject a mock; the real survey/v2 implementation
// lands in C04 (auth login) where the first prompt is actually needed.
// Keeping the interface in its own package now means every later sprint
// imports a stable type, not a moving target.
package prompter

// Prompter abstracts user-interactive prompts. Every method must return
// quickly (no hanging on stdin) and must surface a clear error when the
// surrounding IOStreams reports NeverPrompt() — typically the test path.
//
// All Select / MultiSelect methods take options as a []string and return
// indices (or slices of indices) rather than option strings, matching
// survey/v2's semantics so adapters stay thin.
type Prompter interface {
	// Confirm asks a yes/no question with a default. Returns the chosen
	// value, or an error if the user aborted (Ctrl-C) or the stream was
	// non-interactive.
	Confirm(message string, defaultValue bool) (bool, error)

	// Input asks for a free-form string with a default value. Empty input
	// returns the default.
	Input(message, defaultValue string) (string, error)

	// Password reads a secret with echo suppressed. The returned string is
	// untrimmed so users can paste tokens with surrounding whitespace if
	// their input method requires.
	Password(message string) (string, error)

	// Select displays a single-choice menu and returns the chosen index.
	Select(message, defaultValue string, options []string) (int, error)

	// MultiSelect displays a checkbox menu. Returns the chosen indices
	// in their original options order.
	MultiSelect(message string, defaults, options []string) ([]int, error)
}

// Aborted is returned by implementations when the user interrupts a
// prompt (Ctrl-C, EOF on stdin). Commands typically translate this to a
// clean non-zero exit with a "cancelled" message rather than a stack trace.
type Aborted struct{}

func (Aborted) Error() string { return "prompter: user aborted" }

// NotInteractive is returned by implementations when the active IOStreams
// reports NeverPrompt(). Commands should translate it to a "this command
// requires an interactive terminal; pass --<flag> instead" message so the
// user knows which non-interactive escape exists.
type NotInteractive struct {
	Reason string
}

func (e NotInteractive) Error() string {
	if e.Reason == "" {
		return "prompter: no interactive terminal available"
	}
	return "prompter: " + e.Reason
}
