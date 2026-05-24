// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

// TestDisplayState pins the merged > closed > draft > open precedence
// from audit-I7. The closed-draft and merged-draft rows are the two
// audit reproducers; the rest are routine.
func TestDisplayState(t *testing.T) {
	mergedAt := time.Now()
	cases := []struct {
		name string
		pr   pulls.PR
		want string
	}{
		{name: "nil-like via zero value", pr: pulls.PR{State: ""}, want: ""},
		{name: "plain open", pr: pulls.PR{State: "open"}, want: "open"},
		{name: "plain closed", pr: pulls.PR{State: "closed"}, want: "closed"},
		{name: "draft open", pr: pulls.PR{State: "open", Draft: true}, want: "draft"},
		{
			name: "closed draft (audit-I7) — must be closed not draft",
			pr:   pulls.PR{State: "closed", Draft: true},
			want: "closed",
		},
		{
			name: "merged via Merged flag",
			pr:   pulls.PR{State: "closed", Merged: true},
			want: "merged",
		},
		{
			name: "merged via MergedAt only",
			pr:   pulls.PR{State: "closed", MergedAt: &mergedAt},
			want: "merged",
		},
		{
			name: "merged draft — merged wins",
			pr:   pulls.PR{State: "closed", Draft: true, Merged: true},
			want: "merged",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DisplayState(&tc.pr)
			if got != tc.want {
				t.Errorf("DisplayState: got %q, want %q (pr=%+v)", got, tc.want, tc.pr)
			}
		})
	}
}

func TestDisplayStateNil(t *testing.T) {
	if got := DisplayState(nil); got != "" {
		t.Errorf("DisplayState(nil) = %q, want \"\"", got)
	}
}
