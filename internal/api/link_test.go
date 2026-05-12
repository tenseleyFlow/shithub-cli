// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "testing"

func TestParseLinkHeader(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		header string
		want   map[string]string
	}{
		{
			name:   "empty",
			header: "",
			want:   map[string]string{},
		},
		{
			name:   "single next link",
			header: `<https://shithub.sh/api/v1/repos?page=2>; rel="next"`,
			want: map[string]string{
				"next": "https://shithub.sh/api/v1/repos?page=2",
			},
		},
		{
			name: "full pagination set",
			header: `<https://shithub.sh/api/v1/repos?page=2>; rel="next", ` +
				`<https://shithub.sh/api/v1/repos?page=5>; rel="last", ` +
				`<https://shithub.sh/api/v1/repos?page=1>; rel="first"`,
			want: map[string]string{
				"next":  "https://shithub.sh/api/v1/repos?page=2",
				"last":  "https://shithub.sh/api/v1/repos?page=5",
				"first": "https://shithub.sh/api/v1/repos?page=1",
			},
		},
		{
			name:   "unquoted rel value tolerated",
			header: `<https://shithub.sh/x?page=2>; rel=next`,
			want: map[string]string{
				"next": "https://shithub.sh/x?page=2",
			},
		},
		{
			name:   "extra params ignored",
			header: `<https://shithub.sh/x?page=2>; foo=bar; rel="next"; title="page 2"`,
			want: map[string]string{
				"next": "https://shithub.sh/x?page=2",
			},
		},
		{
			name:   "URL with comma preserved",
			header: `<https://shithub.sh/x?q=a,b&page=2>; rel="next"`,
			want: map[string]string{
				"next": "https://shithub.sh/x?q=a,b&page=2",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseLinkHeader(tc.header)
			if len(got) != len(tc.want) {
				t.Fatalf("len: want %d got %d (%v)", len(tc.want), len(got), got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%q: want %q got %q", k, v, got[k])
				}
			}
		})
	}
}

func TestParseLinkHeaderMalformedSkippedNotCrash(t *testing.T) {
	t.Parallel()
	// Bad entry alongside a valid one — valid entry survives.
	header := `not-a-link, <https://shithub.sh/x>; rel="next"`
	got := ParseLinkHeader(header)
	if got["next"] != "https://shithub.sh/x" {
		t.Errorf("malformed entry ate the valid one: %v", got)
	}
}
