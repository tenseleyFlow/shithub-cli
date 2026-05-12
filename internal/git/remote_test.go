// SPDX-License-Identifier: AGPL-3.0-or-later

package git

import "testing"

func TestParseRemoteURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		in          string
		wantHost    string
		wantOwner   string
		wantRepo    string
		expectError bool
	}{
		{
			name:      "https with .git",
			in:        "https://shithub.sh/mf/shithub-cli.git",
			wantHost:  "shithub.sh",
			wantOwner: "mf",
			wantRepo:  "shithub-cli",
		},
		{
			name:      "https without .git",
			in:        "https://shithub.sh/mf/shithub-cli",
			wantHost:  "shithub.sh",
			wantOwner: "mf",
			wantRepo:  "shithub-cli",
		},
		{
			name:      "http (dev-only)",
			in:        "http://localhost:8080/mf/repo",
			wantHost:  "localhost",
			wantOwner: "mf",
			wantRepo:  "repo",
		},
		{
			name:      "scp-style ssh",
			in:        "git@shithub.sh:mf/shithub-cli.git",
			wantHost:  "shithub.sh",
			wantOwner: "mf",
			wantRepo:  "shithub-cli",
		},
		{
			name:      "ssh url form",
			in:        "ssh://git@shithub.sh/mf/shithub-cli.git",
			wantHost:  "shithub.sh",
			wantOwner: "mf",
			wantRepo:  "shithub-cli",
		},
		{
			name:      "host case normalized",
			in:        "https://SHITHUB.SH/mf/cli",
			wantHost:  "shithub.sh",
			wantOwner: "mf",
			wantRepo:  "cli",
		},
		{
			name:        "empty input",
			in:          "",
			expectError: true,
		},
		{
			name:        "unsupported scheme",
			in:          "ftp://shithub.sh/mf/cli",
			expectError: true,
		},
		{
			name:        "too few segments",
			in:          "https://shithub.sh/mf",
			expectError: true,
		},
		{
			name:        "too many segments",
			in:          "https://shithub.sh/mf/cli/extra",
			expectError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRemoteURL(tc.in)
			if tc.expectError {
				if err == nil {
					t.Fatalf("want error for %q, got Remote=%+v", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got.Host != tc.wantHost {
				t.Errorf("Host: want %q got %q", tc.wantHost, got.Host)
			}
			if got.Owner != tc.wantOwner {
				t.Errorf("Owner: want %q got %q", tc.wantOwner, got.Owner)
			}
			if got.Repo != tc.wantRepo {
				t.Errorf("Repo: want %q got %q", tc.wantRepo, got.Repo)
			}
		})
	}
}

func TestRemoteString(t *testing.T) {
	t.Parallel()
	if got := (Remote{Owner: "mf", Repo: "cli"}).String(); got != "mf/cli" {
		t.Errorf("String: got %q", got)
	}
	if got := (Remote{Owner: "", Repo: "cli"}).String(); got != "" {
		t.Errorf("incomplete Remote should yield empty, got %q", got)
	}
}
