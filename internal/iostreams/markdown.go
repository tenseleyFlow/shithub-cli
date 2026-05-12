// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import (
	"fmt"
	"os"

	"github.com/charmbracelet/glamour"
)

// EnvGlamourStyle is the override for glamour's stylesheet selection.
// Matches gh's variable so users carrying GLAMOUR_STYLE across both
// binaries get the same behavior.
const EnvGlamourStyle = "GLAMOUR_STYLE"

// RenderMarkdown returns a terminal-styled rendering of the markdown
// source. When color is disabled or stdout is not a TTY, the original
// markdown is returned unchanged so pipes get clean text and copy/paste
// preserves the source.
//
// Width follows the current terminal (TerminalWidth) so wrapping looks
// right at the active size. GLAMOUR_STYLE env var overrides the
// auto-selected theme; the default chooses dark/light based on the
// terminal's reported background where possible, falling back to dark.
func (s *IOStreams) RenderMarkdown(md string) (string, error) {
	if !s.colorEnabled || !s.stdoutTTY {
		return md, nil
	}

	style := os.Getenv(EnvGlamourStyle)
	if style == "" {
		// "auto" inspects COLORFGBG and similar hints; falls back to "dark"
		// when ambiguous. Good default for our case.
		style = "auto"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithWordWrap(s.TerminalWidth()),
		glamour.WithEmoji(),
	)
	if err != nil {
		return "", fmt.Errorf("markdown: build renderer: %w", err)
	}

	out, err := r.Render(md)
	if err != nil {
		return "", fmt.Errorf("markdown: render: %w", err)
	}
	return out, nil
}
