// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import (
	"time"

	"github.com/briandowns/spinner"
)

// spinnerFrameInterval is the per-frame delay for the spinner animation.
// 100ms matches gh and feels responsive without being chatty in screen
// captures.
const spinnerFrameInterval = 100 * time.Millisecond

// spinnerCharset is the spinner glyph set. Index 14 is the braille-dot
// rotating pattern (the gh default). Picked once; never bikeshedded.
const spinnerCharset = 14

// StartSpinner shows a small animated indicator on stderr with the given
// label. No-op when stdout is not a TTY (so log scrapers don't see
// thousands of cursor-position escapes) or when a pager / non-color
// rendering mode is active.
//
// Pair with StopSpinner via defer:
//
//	stop := ios.StartSpinner("looking up repo...")
//	defer stop()
//
// stop is always safe to call, even when no spinner was actually started.
func (s *IOStreams) StartSpinner(label string) (stop func()) {
	if !s.stdoutTTY || s.pager.cmd != nil {
		return func() {}
	}

	sp := spinner.New(
		spinner.CharSets[spinnerCharset], spinnerFrameInterval,
		spinner.WithWriter(s.ErrOut),
		spinner.WithSuffix(" "+label),
	)
	sp.Start()
	return func() {
		sp.Stop()
		// briandowns/spinner leaves the cursor on the spinner line; print
		// a CR to reset so the next stderr write starts cleanly.
		_, _ = s.ErrOut.Write([]byte("\r"))
	}
}
