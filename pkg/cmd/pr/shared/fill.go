// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"fmt"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/git"
)

// FillOptions configures the --fill / --fill-first / --fill-verbose
// helpers. Mode picks which fill strategy to apply; revRange is the
// commit range to read (typically "base..HEAD").
type FillOptions struct {
	Mode     FillMode
	RevRange string
}

// FillMode is the strategy selector.
type FillMode int

const (
	// FillNone is the default — no autofill.
	FillNone FillMode = iota
	// FillStandard mirrors gh's `--fill`: first commit subject becomes
	// title, remaining commit subjects + bodies become PR body.
	FillStandard
	// FillFirst mirrors gh's `--fill-first`: first commit's subject is
	// the title, first commit's body is the PR body (other commits
	// untouched).
	FillFirst
	// FillVerbose mirrors gh's `--fill-verbose`: first commit subject is
	// the title; PR body concatenates every commit's subject AND body.
	FillVerbose
)

// Fill reads commits from `runner` over `opts.RevRange` and returns
// (title, body) per the chosen FillMode. Empty range returns empties so
// the caller can fall through to prompts / flag values.
func Fill(runner git.Runner, dir string, opts FillOptions) (title, body string, err error) {
	if opts.Mode == FillNone || opts.RevRange == "" {
		return "", "", nil
	}
	subjects, err := git.LogSubjects(runner, dir, opts.RevRange)
	if err != nil {
		return "", "", fmt.Errorf("fill: log subjects: %w", err)
	}
	if len(subjects) == 0 {
		return "", "", nil
	}
	title = subjects[0]

	switch opts.Mode {
	case FillFirst:
		body, err = git.LogBody(runner, dir, opts.RevRange)
		if err != nil {
			return "", "", fmt.Errorf("fill: log body: %w", err)
		}
	case FillStandard:
		// gh's --fill puts the remaining subjects (one per line) into the body.
		if len(subjects) > 1 {
			body = strings.Join(subjects[1:], "\n")
		}
	case FillVerbose:
		// --fill-verbose: every commit's full message (subject + body)
		// stitched together. Use `--format=%B -z` so we get NUL-separated
		// chunks we can split cleanly.
		full, ferr := runner.Output(dir, "log", "--reverse", "-z", "--format=%B", opts.RevRange)
		if ferr != nil {
			return "", "", fmt.Errorf("fill: log full: %w", ferr)
		}
		body = strings.Join(splitNUL(string(full)), "\n\n")
	}
	return title, strings.TrimSpace(body), nil
}

// splitNUL trims the trailing NUL terminator git -z adds and splits on
// the remaining NUL delimiters. Empty parts are dropped.
func splitNUL(s string) []string {
	s = strings.TrimRight(s, "\x00")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
