// SPDX-License-Identifier: AGPL-3.0-or-later

// Package tableprinter renders rows of cells in two modes:
//
//   - TTY mode: human-readable, column-aligned, width-aware (truncates
//     cells that overflow the terminal). Optional headers.
//   - Pipe mode: tab-separated values, one row per line, no truncation —
//     so `shithub ... | awk '{print $1}'` works the way users expect.
//
// Mode is selected per-printer at construction time so tests can drive
// both paths without touching real TTY state.
package tableprinter

import (
	"fmt"
	"io"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/text"
)

// columnSeparator is the rendered gap between columns in TTY mode.
// Two spaces is the gh convention; visually distinct from a single space
// run inside a cell.
const columnSeparator = "  "

// Printer accumulates rows in memory and flushes them all at once on
// Render. We buffer because column widths can only be computed after all
// rows are known.
type Printer struct {
	out       io.Writer
	isTTY     bool
	maxWidth  int
	headers   []string
	rows      [][]string
	colWidths []int
}

// New returns a Printer writing to out. isTTY selects column-aligned vs
// TSV mode; maxWidth caps the rendered line length in TTY mode (callers
// typically pass IOStreams.TerminalWidth()).
//
// If isTTY is false maxWidth is ignored — TSV output never truncates.
func New(out io.Writer, isTTY bool, maxWidth int) *Printer {
	return &Printer{out: out, isTTY: isTTY, maxWidth: maxWidth}
}

// AddHeader registers the column headers. Optional — call only if you
// want a header row in TTY mode. Headers are NOT emitted in TSV mode (so
// piping into `awk` is unambiguous about field positions).
func (p *Printer) AddHeader(headers ...string) {
	p.headers = headers
}

// AddRow appends a row. Cell count should match across rows; shorter
// rows are padded with empty cells on Render. Cells may contain ANSI
// escape sequences; column widths are measured by visible width via
// internal/text.VisibleLen.
func (p *Printer) AddRow(cells ...string) {
	p.rows = append(p.rows, cells)
}

// Render flushes accumulated rows to the writer.
func (p *Printer) Render() error {
	if len(p.rows) == 0 && len(p.headers) == 0 {
		return nil
	}
	if !p.isTTY {
		return p.renderTSV()
	}
	return p.renderTTY()
}

// renderTSV writes one row per line, tab-separated, no header. Cells
// containing tabs or newlines have those characters replaced with a
// single space — preserving line/column semantics for downstream pipes.
func (p *Printer) renderTSV() error {
	for _, row := range p.rows {
		for i, cell := range row {
			if i > 0 {
				if _, err := io.WriteString(p.out, "\t"); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(p.out, sanitizeTSV(cell)); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(p.out, "\n"); err != nil {
			return err
		}
	}
	return nil
}

func sanitizeTSV(s string) string {
	r := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return r.Replace(s)
}

// renderTTY computes per-column widths, optionally writes the header,
// then writes each row truncated to fit the terminal.
func (p *Printer) renderTTY() error {
	p.computeWidths()
	// Cap total width if necessary by trimming the last column.
	if p.maxWidth > 0 {
		p.shrinkToFit()
	}

	if len(p.headers) > 0 {
		if err := p.writeRow(p.headers); err != nil {
			return err
		}
	}
	for _, row := range p.rows {
		if err := p.writeRow(row); err != nil {
			return err
		}
	}
	return nil
}

// computeWidths walks headers + rows and records the widest visible
// cell per column. Widths are computed once per Render call.
func (p *Printer) computeWidths() {
	cols := 0
	if len(p.headers) > cols {
		cols = len(p.headers)
	}
	for _, row := range p.rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	p.colWidths = make([]int, cols)
	measure := func(row []string) {
		for i, cell := range row {
			if w := text.VisibleLen(cell); w > p.colWidths[i] {
				p.colWidths[i] = w
			}
		}
	}
	measure(p.headers)
	for _, row := range p.rows {
		measure(row)
	}
}

// shrinkToFit reduces the last column's width until total <= maxWidth.
// Earlier columns are preserved (typically: number, state, then title).
// If the last column gets squeezed below 4 columns we stop; truncation
// is better than rendering an unreadable narrow strip.
func (p *Printer) shrinkToFit() {
	if len(p.colWidths) == 0 {
		return
	}
	total := 0
	for _, w := range p.colWidths {
		total += w
	}
	total += (len(p.colWidths) - 1) * len(columnSeparator)
	if total <= p.maxWidth {
		return
	}
	overflow := total - p.maxWidth
	last := len(p.colWidths) - 1
	if newWidth := p.colWidths[last] - overflow; newWidth >= 4 {
		p.colWidths[last] = newWidth
	} else {
		p.colWidths[last] = 4
	}
}

func (p *Printer) writeRow(row []string) error {
	for i := 0; i < len(p.colWidths); i++ {
		if i > 0 {
			if _, err := io.WriteString(p.out, columnSeparator); err != nil {
				return err
			}
		}
		cell := ""
		if i < len(row) {
			cell = row[i]
		}
		cell = text.Truncate(p.colWidths[i], cell)
		if _, err := io.WriteString(p.out, cell); err != nil {
			return err
		}
		// Pad to column width with visible-aware padding.
		if pad := p.colWidths[i] - text.VisibleLen(cell); pad > 0 && i < len(p.colWidths)-1 {
			if _, err := fmt.Fprint(p.out, strings.Repeat(" ", pad)); err != nil {
				return err
			}
		}
	}
	if _, err := io.WriteString(p.out, "\n"); err != nil {
		return err
	}
	return nil
}
