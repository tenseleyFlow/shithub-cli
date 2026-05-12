// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"strings"
	"testing"
)

func TestPlaceholderSubstitute(t *testing.T) {
	t.Parallel()
	s := placeholderSpec{Owner: "o", Repo: "r", Branch: "trunk"}

	cases := map[string]string{
		"repos/{owner}/{repo}":                   "repos/o/r",
		"repos/{owner}/{repo}/branches/{branch}": "repos/o/r/branches/trunk",
		"user":                                   "user",
	}
	for in, want := range cases {
		got, err := s.substitute(in)
		if err != nil {
			t.Errorf("substitute(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("substitute(%q): want %q got %q", in, want, got)
		}
	}
}

func TestPlaceholderMissingValueErrors(t *testing.T) {
	t.Parallel()
	s := placeholderSpec{} // empty
	_, err := s.substitute("repos/{owner}/{repo}")
	if err == nil {
		t.Fatal("expected error when {owner} unresolved")
	}
	if !strings.Contains(err.Error(), "-R") {
		t.Errorf("error should hint at -R, got: %v", err)
	}
}

func TestSplitRepoSpec(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in          string
		wantOwner   string
		wantRepo    string
		expectError bool
	}{
		{in: "owner/repo", wantOwner: "owner", wantRepo: "repo"},
		{in: "ownerOnly", expectError: true},
		{in: "/repo", expectError: true},
		{in: "owner/", expectError: true},
		{in: "a/b/c", expectError: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			o, r, err := splitRepoSpec(tc.in)
			if tc.expectError {
				if err == nil {
					t.Errorf("want error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("split: %v", err)
			}
			if o != tc.wantOwner || r != tc.wantRepo {
				t.Errorf("split(%q): got (%q,%q)", tc.in, o, r)
			}
		})
	}
}

func TestResolvePlaceholdersFromFlag(t *testing.T) {
	t.Parallel()
	got, err := resolvePlaceholders("o/r", "shithub.sh")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Owner != "o" || got.Repo != "r" {
		t.Errorf("got %+v", got)
	}
}

func TestResolvePlaceholdersFromEnv(t *testing.T) {
	t.Setenv(EnvRepo, "envowner/envrepo")
	got, err := resolvePlaceholders("", "shithub.sh")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.Owner != "envowner" || got.Repo != "envrepo" {
		t.Errorf("got %+v", got)
	}
}

func TestResolvePlaceholdersInvalidFlag(t *testing.T) {
	t.Parallel()
	_, err := resolvePlaceholders("notvalid", "")
	if err == nil {
		t.Fatal("expected error on malformed -R")
	}
}
