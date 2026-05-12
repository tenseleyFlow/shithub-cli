// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import "testing"

func TestParseRangeForms(t *testing.T) {
	cases := []struct {
		in   string
		want Range
	}{
		{"10", Range{HasLower: true, HasUpper: true, Lower: 10, Upper: 10}},
		{">10", Range{HasLower: true, Lower: 11}},
		{">=10", Range{HasLower: true, Lower: 10}},
		{"<10", Range{HasUpper: true, Upper: 9}},
		{"<=10", Range{HasUpper: true, Upper: 10}},
		{"10..50", Range{HasLower: true, HasUpper: true, Lower: 10, Upper: 50}},
		{"*..10", Range{HasUpper: true, Upper: 10}},
		{"10..*", Range{HasLower: true, Lower: 10}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseRange(tc.in)
			if err != nil {
				t.Fatalf("ParseRange(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseRange(%q) = %+v; want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseRangeErrors(t *testing.T) {
	bad := []string{"", "abc", ">x", "*..*", "..", "1..x", "x..1"}
	for _, in := range bad {
		if _, err := ParseRange(in); err == nil {
			t.Errorf("ParseRange(%q): want error", in)
		}
	}
}

func TestRangeString(t *testing.T) {
	cases := []struct {
		r    Range
		want string
	}{
		{Range{}, ""},
		{Range{HasLower: true, HasUpper: true, Lower: 5, Upper: 5}, "5"},
		{Range{HasLower: true, HasUpper: true, Lower: 1, Upper: 10}, "1..10"},
		{Range{HasLower: true, Lower: 5}, ">=5"},
		{Range{HasUpper: true, Upper: 5}, "<=5"},
	}
	for _, tc := range cases {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("%+v.String() = %q; want %q", tc.r, got, tc.want)
		}
	}
}

// TestRangeRoundTripCanonicalization locks in the audit #148 decision:
// exclusive bounds (`>N`, `<N`) round-trip into their inclusive form
// rather than back to the original string. Anyone tempted to "fix" this
// must update the doc comment and this test together.
func TestRangeRoundTripCanonicalization(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{">10", ">=11"},
		{"<10", "<=9"},
		{">=10", ">=10"},
		{"<=10", "<=10"},
		{"10", "10"},
		{"10..20", "10..20"},
		{"*..10", "<=10"},
		{"10..*", ">=10"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			r, err := ParseRange(tc.in)
			if err != nil {
				t.Fatalf("ParseRange(%q): %v", tc.in, err)
			}
			if got := r.String(); got != tc.want {
				t.Errorf("ParseRange(%q).String() = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestComposeQueryQuotesWhitespace(t *testing.T) {
	got := ComposeQuery("octocat", Qualifier{Key: "label", Value: "good first issue"})
	want := `octocat label:"good first issue"`
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

// TestComposeQueryEscapesInnerQuotes covers audit #149: a qualifier
// value with whitespace AND embedded double-quotes must escape the
// inner quotes so the server's tokenizer sees one qualifier, not three
// half-broken tokens.
func TestComposeQueryEscapesInnerQuotes(t *testing.T) {
	got := ComposeQuery("", Qualifier{Key: "label", Value: `has "quotes" here`})
	want := `label:"has \"quotes\" here"`
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestComposeQuerySkipsEmpty(t *testing.T) {
	got := ComposeQuery("", Qualifier{Key: "stars", Value: ">10"}, Qualifier{Key: "", Value: "x"}, Qualifier{Key: "x", Value: ""})
	want := "stars:>10"
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}

func TestBoolQualifier(t *testing.T) {
	tru, fal := true, false
	if got := BoolQualifier("archived", &tru).Value; got != "true" {
		t.Errorf("true: %q", got)
	}
	if got := BoolQualifier("archived", &fal).Value; got != "false" {
		t.Errorf("false: %q", got)
	}
	if got := BoolQualifier("archived", nil).Key; got != "" {
		t.Errorf("nil: %q", got)
	}
}
