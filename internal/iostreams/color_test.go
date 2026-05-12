// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import (
	"strings"
	"testing"
)

func TestColorDisabledReturnsRawString(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	// SetColorEnabled(false) is the Test() default; confirmed by an explicit
	// assertion so a future Test() change can't silently flip this.
	if s.ColorEnabled() {
		t.Fatal("Test() should default color off")
	}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Red", s.Red("hello"), "hello"},
		{"Green", s.Green("hello"), "hello"},
		{"Yellow", s.Yellow("hello"), "hello"},
		{"Blue", s.Blue("hello"), "hello"},
		{"Magenta", s.Magenta("hello"), "hello"},
		{"Cyan", s.Cyan("hello"), "hello"},
		{"Gray", s.Gray("hello"), "hello"},
		{"Bold", s.Bold("hello"), "hello"},
		{"Italic", s.Italic("hello"), "hello"},
		{"Underline", s.Underline("hello"), "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("want raw %q, got %q", tc.want, tc.got)
			}
		})
	}
}

func TestColorEnabledPathDelegatesToLipgloss(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	s.SetColorEnabled(true)

	// We can't assert on ANSI bytes here: lipgloss probes the runtime
	// terminal for color-profile support and may downgrade to plain output
	// when invoked from `go test` without a real terminal. The contract
	// this test enforces is narrower: the call path must not drop or
	// mutate the payload, and must not panic.
	red := s.Red("payload")
	if !strings.Contains(red, "payload") {
		t.Errorf("Red(payload) should preserve payload, got %q", red)
	}
}

func TestSuccessIconFallback(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	if got := s.SuccessIcon(); got != "v" {
		t.Errorf("SuccessIcon plain: want %q got %q", "v", got)
	}
	if got := s.FailureIcon(); got != "X" {
		t.Errorf("FailureIcon plain: want %q got %q", "X", got)
	}
	if got := s.WarningIcon(); got != "!" {
		t.Errorf("WarningIcon plain: want %q got %q", "!", got)
	}
}

func TestSuccessIconColored(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	s.SetColorEnabled(true)
	if !strings.Contains(s.SuccessIcon(), "✓") {
		t.Errorf("SuccessIcon colored should contain ✓, got %q", s.SuccessIcon())
	}
	if !strings.Contains(s.FailureIcon(), "✗") {
		t.Errorf("FailureIcon colored should contain ✗, got %q", s.FailureIcon())
	}
}
