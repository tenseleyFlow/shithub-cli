// SPDX-License-Identifier: AGPL-3.0-or-later

package tableprinter

import (
	"bytes"
	"strings"
	"testing"
)

func TestTSVModeOmitsHeader(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := New(&buf, false, 0)
	p.AddHeader("NUMBER", "TITLE")
	p.AddRow("42", "fix the thing")
	p.AddRow("7", "tidy up docs")

	if err := p.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, "NUMBER") {
		t.Errorf("TSV mode should not emit header row, got: %q", got)
	}
	wantLines := []string{
		"42\tfix the thing",
		"7\ttidy up docs",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("missing TSV line %q in output %q", want, got)
		}
	}
}

func TestTSVSanitizesTabsAndNewlines(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := New(&buf, false, 0)
	p.AddRow("a\tb", "line1\nline2")

	if err := p.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	// Sanitizer replaces embedded tabs and newlines with spaces. The single
	// separator tab between cells is the only legitimate \t in the output.
	if strings.Count(got, "\t") != 1 {
		t.Errorf("expected exactly one separator tab, got %q", got)
	}
}

func TestTTYModeEmitsHeader(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := New(&buf, true, 80)
	p.AddHeader("NUMBER", "TITLE")
	p.AddRow("42", "fix the thing")

	if err := p.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "NUMBER") {
		t.Errorf("TTY mode should start with header, got: %q", got)
	}
}

func TestTTYColumnAlignment(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := New(&buf, true, 200)
	p.AddRow("1", "short")
	p.AddRow("12345", "long")

	if err := p.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), got)
	}
	// First column padded to "12345" width (5) means line 1 starts with
	// "1    " (one digit + 4 spaces) followed by columnSeparator.
	if !strings.HasPrefix(lines[0], "1    "+columnSeparator) {
		t.Errorf("column not padded: %q", lines[0])
	}
}

func TestTTYShrinksLastColumn(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	// maxWidth tight enough to force the last column to shrink.
	p := New(&buf, true, 20)
	p.AddRow("123", "this title is too long to fit completely in twenty columns")

	if err := p.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	// One row + newline; must contain the ellipsis from text.Truncate.
	if !strings.Contains(got, "…") {
		t.Errorf("expected truncated cell with ellipsis, got %q", got)
	}
	// And line length (visible) should be <= maxWidth.
	line := strings.TrimRight(got, "\n")
	if vis := visibleLen(line); vis > 20 {
		t.Errorf("rendered line exceeds maxWidth: vis=%d line=%q", vis, line)
	}
}

func TestRenderNoRowsNoError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := New(&buf, true, 80)
	if err := p.Render(); err != nil {
		t.Fatalf("empty Render: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty Render should write nothing, got %q", buf.String())
	}
}

func TestShortRowPaddedWithEmptyCells(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	p := New(&buf, true, 80)
	p.AddHeader("A", "B", "C")
	p.AddRow("1") // intentionally short
	p.AddRow("2", "x", "y")

	if err := p.Render(); err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "1") || !strings.Contains(got, "2") {
		t.Errorf("expected both rows present, got %q", got)
	}
}

// visibleLen is a duplicate of internal/text.VisibleLen kept here so the
// table test stays self-contained without importing internal/text from a
// peer internal package's tests.
func visibleLen(s string) int {
	out := 0
	in := false
	for _, r := range s {
		if r == 0x1b {
			in = true
			continue
		}
		if in {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				in = false
			}
			continue
		}
		out++
	}
	return out
}
