// SPDX-License-Identifier: AGPL-3.0-or-later

package text

import (
	"testing"
	"time"
)

func TestTruncate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		width    int
		in, want string
	}{
		{name: "short string unchanged", width: 10, in: "hello", want: "hello"},
		{name: "exact width unchanged", width: 5, in: "hello", want: "hello"},
		{name: "truncate with ellipsis", width: 5, in: "abcdefgh", want: "abcd…"},
		{name: "width zero returns empty", width: 0, in: "abc", want: ""},
		{name: "negative width returns empty", width: -1, in: "abc", want: ""},
		{name: "width one returns first rune", width: 1, in: "abc", want: "a"},
		{name: "unicode payload preserved", width: 3, in: "日本語ABC", want: "日本…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Truncate(tc.width, tc.in); got != tc.want {
				t.Errorf("Truncate(%d, %q): want %q got %q", tc.width, tc.in, tc.want, got)
			}
		})
	}
}

func TestTruncatePreservesANSI(t *testing.T) {
	t.Parallel()
	// Colored "abcdefgh" wrapped in red SGR; visible width 8.
	in := "\x1b[31mabcdefgh\x1b[0m"
	got := Truncate(5, in)

	// Visible portion must be "abcd…", and the leading escape must survive.
	if VisibleLen(got) != 5 {
		t.Errorf("VisibleLen: want 5 got %d (%q)", VisibleLen(got), got)
	}
	if !contains(got, "\x1b[31m") {
		t.Errorf("opening escape stripped: %q", got)
	}
}

func TestVisibleLen(t *testing.T) {
	t.Parallel()
	cases := map[string]int{
		"":                   0,
		"abc":                3,
		"\x1b[31mabc\x1b[0m": 3,
		"日本語":                3,
	}
	for in, want := range cases {
		if got := VisibleLen(in); got != want {
			t.Errorf("VisibleLen(%q): want %d got %d", in, want, got)
		}
	}
}

func TestPluralize(t *testing.T) {
	t.Parallel()
	cases := map[int]string{
		0: "0 issues",
		1: "1 issue",
		2: "2 issues",
	}
	for n, want := range cases {
		if got := Pluralize(n, "issue"); got != want {
			t.Errorf("Pluralize(%d, issue): want %q got %q", n, want, got)
		}
	}
}

func TestFormatRelative(t *testing.T) {
	t.Parallel()
	cases := []struct {
		d      time.Duration
		future bool
		want   string
	}{
		{d: 10 * time.Second, want: "just now"},
		{d: 45 * time.Second, want: "45 seconds ago"},
		{d: 5 * time.Minute, want: "5 minutes ago"},
		{d: 1 * time.Minute, want: "1 minute ago"},
		{d: 3 * time.Hour, want: "3 hours ago"},
		{d: 2 * 24 * time.Hour, want: "2 days ago"},
		{d: 10 * 24 * time.Hour, want: "1 week ago"},
		{d: 60 * 24 * time.Hour, want: "2 months ago"},
		{d: 800 * 24 * time.Hour, want: "2 years ago"},
		{d: 5 * time.Minute, future: true, want: "in 5 minutes"},
	}
	for _, tc := range cases {
		if got := formatRelative(tc.d, tc.future); got != tc.want {
			t.Errorf("formatRelative(%v, future=%v): want %q got %q", tc.d, tc.future, tc.want, got)
		}
	}
}

func TestRelativeTimeZero(t *testing.T) {
	t.Parallel()
	if got := RelativeTime(time.Time{}); got != "" {
		t.Errorf("zero time should return empty, got %q", got)
	}
}

func TestIndent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		prefix string
		in     string
		want   string
	}{
		{name: "empty stays empty", prefix: "> ", in: "", want: ""},
		{name: "single line", prefix: "> ", in: "hi", want: "> hi"},
		{name: "multi line", prefix: "> ", in: "a\nb\nc", want: "> a\n> b\n> c"},
		{name: "trailing newline preserved", prefix: "> ", in: "a\nb\n", want: "> a\n> b\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Indent(tc.prefix, tc.in); got != tc.want {
				t.Errorf("Indent: want %q got %q", tc.want, got)
			}
		})
	}
}

func TestRemoveExcessiveWhitespace(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"a\n\nb", "a\n\nb"},
		{"a\n\n\nb", "a\n\nb"},
		{"a\n\n\n\n\nb", "a\n\nb"},
	}
	for _, tc := range cases {
		if got := RemoveExcessiveWhitespace(tc.in); got != tc.want {
			t.Errorf("RemoveExcessiveWhitespace(%q): want %q got %q", tc.in, tc.want, got)
		}
	}
}

func contains(haystack, needle string) bool {
	return indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	n, h := len(needle), len(haystack)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= h; i++ {
		if haystack[i:i+n] == needle {
			return i
		}
	}
	return -1
}
