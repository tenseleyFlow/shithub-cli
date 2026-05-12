// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import "testing"

func TestNormalizeColor(t *testing.T) {
	cases := map[string]struct {
		out string
		ok  bool
	}{
		"abc":     {"aabbcc", true},
		"#abc":    {"aabbcc", true},
		"AbCdEf":  {"abcdef", true},
		"#ff0000": {"ff0000", true},
		"  abc  ": {"aabbcc", true},
		"red":     {"", false},
		"#xyz":    {"", false},
		"":        {"", false},
		"abcd":    {"", false},
		"abcdefg": {"", false},
	}
	for in, want := range cases {
		got, err := NormalizeColor(in)
		if want.ok && err != nil {
			t.Errorf("NormalizeColor(%q) unexpected error: %v", in, err)
			continue
		}
		if !want.ok && err == nil {
			t.Errorf("NormalizeColor(%q) expected error", in)
			continue
		}
		if want.ok && got != want.out {
			t.Errorf("NormalizeColor(%q): got %q want %q", in, got, want.out)
		}
	}
}
