// SPDX-License-Identifier: AGPL-3.0-or-later

// Package text exposes small formatting helpers used by table-printer,
// JSON template funcs, and command-level output. Every helper here is
// pure (no I/O, no globals beyond constants) so callers can compose
// freely and tests can run in parallel.
package text

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ansiCSIRegexp matches ANSI CSI escape sequences ("\x1b[...m" etc.).
// Used by Truncate so a styled cell doesn't get cut mid-escape, which
// would leave a colored cursor across the rest of the row.
var ansiCSIRegexp = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// Truncate cuts s to at most width visible columns, appending an ellipsis
// if truncation occurred. ANSI escape sequences are not counted toward
// width and are preserved through the cut so styling survives.
//
// width <= 0 returns the empty string. width == 1 truncates to a single
// character (no ellipsis room).
func Truncate(width int, s string) string {
	if width <= 0 {
		return ""
	}
	visible := stripANSI(s)
	if len([]rune(visible)) <= width {
		return s
	}
	if width == 1 {
		// Edge case: a single column can't fit an ellipsis; emit the first rune.
		return string([]rune(visible)[:1])
	}

	// Walk s rune-by-rune, copying everything through to width-1 visible
	// runes while preserving any escape sequences we encounter along the way.
	var (
		out   strings.Builder
		count int
		i     int
	)
	r := []rune(s)
	for i < len(r) && count < width-1 {
		if r[i] == 0x1b && i+1 < len(r) && r[i+1] == '[' {
			// Copy the full CSI sequence (terminator is a letter).
			start := i
			i += 2
			for i < len(r) {
				c := r[i]
				i++
				if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
					break
				}
			}
			out.WriteString(string(r[start:i]))
			continue
		}
		out.WriteRune(r[i])
		count++
		i++
	}
	out.WriteRune('…')
	return out.String()
}

// stripANSI returns s with all CSI escape sequences removed.
func stripANSI(s string) string {
	return ansiCSIRegexp.ReplaceAllString(s, "")
}

// VisibleLen returns the visible (ANSI-stripped) rune count of s. Useful
// when callers need to align columns containing colored cells.
func VisibleLen(s string) int {
	return len([]rune(stripANSI(s)))
}

// Pluralize returns "N noun" or "N nouns" using a simple +s rule.
// For irregular plurals, format the result yourself; this helper exists
// to remove the most common boilerplate (Issue/Issues, Hour/Hours, ...).
func Pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// RelativeTime returns a coarse "N units ago" string suitable for table
// cells: "just now", "5 minutes ago", "3 hours ago", "2 days ago",
// "3 weeks ago", "11 months ago", "2 years ago". Negative durations
// (future timestamps — clock skew) render as "in N units".
func RelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	d := now.Sub(t)
	future := false
	if d < 0 {
		future = true
		d = -d
	}
	return formatRelative(d, future)
}

// formatRelative is the unit-decision core extracted so tests can drive
// it deterministically without faking time.Now.
func formatRelative(d time.Duration, future bool) string {
	const (
		minute = 60 * time.Second
		hour   = 60 * minute
		day    = 24 * hour
		week   = 7 * day
		month  = 30 * day // approximate; matches GitHub's display
		year   = 365 * day
	)

	pick := func(n int, unit string) string {
		s := Pluralize(n, unit)
		if future {
			return "in " + s
		}
		return s + " ago"
	}

	switch {
	case d < 30*time.Second && !future:
		return "just now"
	case d < minute:
		return pick(int(d.Seconds()), "second")
	case d < hour:
		return pick(int(d.Minutes()), "minute")
	case d < day:
		return pick(int(d.Hours()), "hour")
	case d < week:
		return pick(int(d/day), "day")
	case d < month:
		return pick(int(d/week), "week")
	case d < year:
		return pick(int(d/month), "month")
	default:
		return pick(int(d/year), "year")
	}
}

// Indent prepends prefix to every line of s. Trailing newlines are
// preserved as-is (so wrapping a body that already ends in \n doesn't
// emit a stray indented blank line).
func Indent(prefix, s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" && i == len(lines)-1 {
			// Preserve trailing-newline emptiness without re-prefixing.
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// excessiveWhitespace matches three-or-more consecutive blank lines.
var excessiveWhitespace = regexp.MustCompile(`\n{3,}`)

// RemoveExcessiveWhitespace collapses runs of three-or-more blank lines
// down to two. Useful for trimming markdown bodies that round-tripped
// through an editor and grew triple-newline gaps.
func RemoveExcessiveWhitespace(s string) string {
	return excessiveWhitespace.ReplaceAllString(s, "\n\n")
}
