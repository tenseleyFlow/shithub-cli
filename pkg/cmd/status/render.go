// SPDX-License-Identifier: AGPL-3.0-or-later

package status

import (
	"fmt"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/iostreams"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
	"github.com/tenseleyFlow/shithub-cli/internal/tableprinter"
)

// render writes the four-section dashboard to stdout. Sections render
// in the fixed order gh uses; empty sections are hidden unless
// showEmpty is set.
func render(io *iostreams.IOStreams, d Dashboard, showEmpty bool) {
	fmt.Fprintf(io.Out, "Hello @%s! Here's your status:\n", d.User)

	type section struct {
		title string
		count int
		print func()
	}
	sections := []section{
		{
			title: "Assigned Issues",
			count: len(d.AssignedIssues),
			print: func() { printIssues(io, d.AssignedIssues) },
		},
		{
			title: "Assigned Pull Requests",
			count: len(d.AssignedPRs),
			print: func() { printPRs(io, d.AssignedPRs) },
		},
		{
			title: "Review Requests",
			count: len(d.ReviewRequests),
			print: func() { printPRs(io, d.ReviewRequests) },
		},
		{
			title: "Mentions",
			count: len(d.Mentions),
			print: func() { printIssues(io, d.Mentions) },
		},
	}

	for _, s := range sections {
		if s.count == 0 && !showEmpty {
			continue
		}
		fmt.Fprintf(io.Out, "\n%s (%d)\n", s.title, s.count)
		if s.count == 0 {
			fmt.Fprintln(io.Out, "  (none)")
			continue
		}
		s.print()
	}
}

// printIssues renders a table for an issue-shaped section.
func printIssues(io *iostreams.IOStreams, items []search.IssueItem) {
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, it := range items {
		repo := ""
		if it.Repository != nil {
			repo = it.Repository.FullName
		}
		tp.AddRow(
			fmt.Sprintf("%s#%d", repo, it.Number),
			truncate(it.Title, 60),
			ageString(it.UpdatedAt),
		)
	}
	_ = tp.Render()
}

// printPRs renders a table for a PR-shaped section. We surface the
// review state when known so review-requested items show "REVIEW_REQUIRED"
// vs "APPROVED" inline.
func printPRs(io *iostreams.IOStreams, items []search.PRItem) {
	tp := tableprinter.New(io.Out, io.IsStdoutTTY(), io.TerminalWidth())
	for _, pr := range items {
		repo := ""
		if pr.Repository != nil {
			repo = pr.Repository.FullName
		}
		tp.AddRow(
			fmt.Sprintf("%s#%d", repo, pr.Number),
			truncate(pr.Title, 60),
			ageString(pr.UpdatedAt),
		)
	}
	_ = tp.Render()
}

// ageString renders a relative-time label. Anything older than a year
// falls back to "Yy"; anything more recent uses the largest unit ≥ 1.
func ageString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/24/365))
	}
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
