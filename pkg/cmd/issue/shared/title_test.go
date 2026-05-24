// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"strings"
	"testing"
)

// TestValidateTitle covers the audit-I52 reproducer plus the boundary
// trims, empties, and oversize cases. The exact phrasing of the error
// messages is non-load-bearing — these only assert the shape so the
// CLI surfaces a useful message instead of letting hostile titles
// reach the server.
func TestValidateTitle(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantOK     bool
		wantClean  string
		wantSubstr string // expected substring of the error when !wantOK
	}{
		{
			name:      "happy",
			in:        "fix typo in README",
			wantOK:    true,
			wantClean: "fix typo in README",
		},
		{
			name:      "trims surrounding whitespace",
			in:        "  fix typo  ",
			wantOK:    true,
			wantClean: "fix typo",
		},
		{
			name:       "empty rejected",
			in:         "",
			wantSubstr: "required",
		},
		{
			name:       "whitespace-only rejected",
			in:         "   \t  ",
			wantSubstr: "required",
		},
		{
			name:       "newline rejected (audit-I52)",
			in:         "line one\nline two",
			wantSubstr: "single line",
		},
		{
			name:       "CR rejected",
			in:         "title\rcontinued",
			wantSubstr: "single line",
		},
		{
			name:       "null byte rejected",
			in:         "title\x00malicious",
			wantSubstr: "null byte",
		},
		{
			name:       "oversize rejected",
			in:         strings.Repeat("a", MaxTitleLen+1),
			wantSubstr: "limit is",
		},
		{
			name:      "exactly at limit accepted",
			in:        strings.Repeat("a", MaxTitleLen),
			wantOK:    true,
			wantClean: strings.Repeat("a", MaxTitleLen),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateTitle(tc.in)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("ValidateTitle(%q): unexpected error %v", tc.in, err)
				}
				if got != tc.wantClean {
					t.Errorf("clean: got %q, want %q", got, tc.wantClean)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateTitle(%q): want error, got nil (clean=%q)", tc.in, got)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}
