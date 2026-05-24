// SPDX-License-Identifier: AGPL-3.0-or-later

package cmdutil

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ParentRunE returns a RunE suitable for a parent/grouping command that
// only exists to host subcommands. With no args we print the parent
// help (like gh's `gh org` does, exit 0). With an unrecognized arg we
// return an error so the binary exits non-zero — cobra's default routes
// the call through the parent's help text and exits 0, which made
// typos like `shithub org INVALID` falsely look successful (E-audit E16).
//
// audit-I5: subcommand-level "did you mean?" suggestions. cobra's
// suggestion machinery only fires at the root command's auto-error
// path; once execution reaches a parent's RunE the suggestion logic
// is bypassed. We walk the parent's children and Levenshtein-match
// the bad arg ourselves so `shithub pr klose` says "Did you mean
// close?" the same way `shithub reepo` already does.
//
// gh emits "unknown command \"INVALID\" for \"gh org\""; we mirror that
// shape and let the user discover the available subcommands via the
// suggestion line.
func ParentRunE() func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		base := fmt.Sprintf("unknown command %q for %q; run '%s --help' for usage",
			args[0], c.CommandPath(), c.CommandPath())
		if suggestion := suggestSubcommand(c, args[0]); suggestion != "" {
			return fmt.Errorf("%s\n\nDid you mean this?\n\t%s", base, suggestion)
		}
		return fmt.Errorf("%s", base)
	}
}

// suggestSubcommand returns the best-matching subcommand name when
// `want` is within Levenshtein-distance 2 of any visible child, or
// shares a 3+ char prefix. Returns "" when nothing's close enough.
//
// We don't reuse cobra's internal SuggestionsFor because it's
// unexported on *Command outside the auto-error path. The
// implementation is small and matches cobra's `SuggestionsMinimumDistance=2`
// default behavior.
func suggestSubcommand(parent *cobra.Command, want string) string {
	type scored struct {
		name string
		dist int
	}
	var best scored
	best.dist = -1
	wantLower := strings.ToLower(want)
	for _, sub := range parent.Commands() {
		if sub.Hidden {
			continue
		}
		name := sub.Name()
		lower := strings.ToLower(name)
		// Prefix match wins outright — `shithub repo cre` → `create`.
		if len(want) >= 3 && strings.HasPrefix(lower, wantLower) {
			return name
		}
		d := levenshtein(wantLower, lower)
		if d <= 2 && (best.dist == -1 || d < best.dist) {
			best = scored{name: name, dist: d}
		}
		// Also check aliases.
		for _, a := range sub.Aliases {
			al := strings.ToLower(a)
			if len(want) >= 3 && strings.HasPrefix(al, wantLower) {
				return name
			}
			d := levenshtein(wantLower, al)
			if d <= 2 && (best.dist == -1 || d < best.dist) {
				best = scored{name: name, dist: d}
			}
		}
	}
	return best.name
}

// levenshtein returns the edit distance between a and b. Tight loop
// suitable for the ~10-15 children typical of a shithub parent.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
