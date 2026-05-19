// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import "github.com/charmbracelet/lipgloss"

// Color wraps lipgloss style application with the IOStreams' color flag.
// Every public color helper goes through Color so that disabling color
// at the stream level (NO_COLOR, --no-color, etc.) instantly suppresses
// every styled string in the program — no caller can accidentally bypass.
//
// Returns the raw string unchanged when ColorEnabled() is false. This
// matters for pipe-friendly output and for tests that snapshot stdout.
func (s *IOStreams) Color(style lipgloss.Style, str string) string {
	if !s.colorEnabled {
		return str
	}
	return style.Render(str)
}

// Predefined styles. Held as package vars so call sites don't recompute
// lipgloss.NewStyle() on every render. Adding a new semantic color means
// adding a const + a thin method — keep the surface small.
var (
	styleRed       = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleGreen     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleYellow    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleBlue      = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	styleMagenta   = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	styleCyan      = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleGray      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleBold      = lipgloss.NewStyle().Bold(true)
	styleItalic    = lipgloss.NewStyle().Italic(true)
	styleUnderline = lipgloss.NewStyle().Underline(true)
)

// Red styles s as red. Convention: error / failure.
func (s *IOStreams) Red(str string) string { return s.Color(styleRed, str) }

// Green styles s as green. Convention: success / passing.
func (s *IOStreams) Green(str string) string { return s.Color(styleGreen, str) }

// Yellow styles s as yellow. Convention: warning / pending.
func (s *IOStreams) Yellow(str string) string { return s.Color(styleYellow, str) }

// Blue styles s as blue. Convention: info / link.
func (s *IOStreams) Blue(str string) string { return s.Color(styleBlue, str) }

// Magenta styles s as magenta. Convention: highlighting categories.
func (s *IOStreams) Magenta(str string) string { return s.Color(styleMagenta, str) }

// Cyan styles s as cyan. Convention: secondary headings.
func (s *IOStreams) Cyan(str string) string { return s.Color(styleCyan, str) }

// Gray styles s as bright black. Convention: muted / secondary text.
func (s *IOStreams) Gray(str string) string { return s.Color(styleGray, str) }

// Bold styles s bold. Combine with a color by chaining: ios.Bold(ios.Red("X")).
func (s *IOStreams) Bold(str string) string { return s.Color(styleBold, str) }

// Italic styles s italic. Terminal support varies; lipgloss handles fallback.
func (s *IOStreams) Italic(str string) string { return s.Color(styleItalic, str) }

// Underline styles s underlined.
func (s *IOStreams) Underline(str string) string { return s.Color(styleUnderline, str) }

// SuccessIcon returns the Unicode check mark, styled green when color
// is on and plain otherwise. G12 (F39): pre-fix the no-color fallback
// emitted ASCII `v` — gh-compat scripts piping `auth status` through
// grep '✓' missed lines that should have matched. Unicode is the
// canonical glyph; color is the optional styling.
func (s *IOStreams) SuccessIcon() string {
	if s.colorEnabled {
		return s.Green("✓")
	}
	return "✓"
}

// FailureIcon returns the Unicode cross mark with the same color rule.
// See SuccessIcon for the rationale; pre-fix this emitted ASCII `X`.
func (s *IOStreams) FailureIcon() string {
	if s.colorEnabled {
		return s.Red("✗")
	}
	return "✗"
}

// WarningIcon returns the canonical warning glyph (an exclamation in a
// triangle). Yellow when colored; "!" when plain.
func (s *IOStreams) WarningIcon() string {
	if s.colorEnabled {
		return s.Yellow("!")
	}
	return "!"
}
