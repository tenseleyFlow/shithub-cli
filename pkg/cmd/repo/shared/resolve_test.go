// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/git"
)

func TestParseRepoArg(t *testing.T) {
	cases := []struct {
		in    string
		ok    bool
		owner string
		name  string
		host  string
	}{
		{"octo/hello", true, "octo", "hello", ""},
		{"shithub.sh/octo/hello", true, "octo", "hello", "shithub.sh"},
		{"hello", false, "", "", ""},
		{"", false, "", "", ""},
		{"a/b/c/d", false, "", "", ""},
		{"/x", false, "", "", ""},
		// C-audit C17: `.git` suffix is stripped on bare forms so
		// `git remote get-url origin | xargs -I{} shithub ... -R {}`
		// works.
		{"octo/hello.git", true, "octo", "hello", ""},
		{"shithub.sh/octo/hello.git", true, "octo", "hello", "shithub.sh"},
		// C-audit C16: HTTPS URL forms (with and without .git suffix).
		{"https://shithub.sh/octo/hello", true, "octo", "hello", "shithub.sh"},
		{"https://shithub.sh/octo/hello.git", true, "octo", "hello", "shithub.sh"},
		// C-audit C16: SCP-style git@ remote URLs.
		{"git@shithub.sh:octo/hello.git", true, "octo", "hello", "shithub.sh"},
		{"git@shithub.sh:octo/hello", true, "octo", "hello", "shithub.sh"},
		// Defense: malformed URL surfaces a clear error.
		{"https://shithub.sh/onlyowner", false, "", "", ""},
	}
	for _, tc := range cases {
		got, err := ParseRepoArg(tc.in)
		if tc.ok && err != nil {
			t.Errorf("ParseRepoArg(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if !tc.ok && err == nil {
			t.Errorf("ParseRepoArg(%q) expected error", tc.in)
			continue
		}
		if tc.ok && (got.Owner != tc.owner || got.Name != tc.name || got.Host != tc.host) {
			t.Errorf("ParseRepoArg(%q): got %+v", tc.in, got)
		}
	}
}

func TestResolvePrefersFlag(t *testing.T) {
	r := Resolver{RepoFlag: "octo/hello", DefaultHost: "shithub.sh"}
	got, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Owner != "octo" || got.Name != "hello" || got.Host != "shithub.sh" {
		t.Errorf("got %+v", got)
	}
}

func TestResolveFlagHostnameOverridesDefault(t *testing.T) {
	r := Resolver{RepoFlag: "octo/hello", Hostname: "ghe.local", DefaultHost: "shithub.sh"}
	got, _ := r.Resolve()
	if got.Host != "ghe.local" {
		t.Errorf("Hostname not respected: %+v", got)
	}
}

func TestResolveExplicitHostInFlag(t *testing.T) {
	r := Resolver{RepoFlag: "ghe.local/octo/hello", Hostname: "ignored", DefaultHost: "shithub.sh"}
	got, _ := r.Resolve()
	if got.Host != "ghe.local" {
		t.Errorf("parsed host should win: %+v", got)
	}
}

func TestResolveFromGitConfig(t *testing.T) {
	gr, dir := newRepo(t)
	if err := git.SetConfig(gr, dir, "shithub.default-repo", "shithub.sh:octo/hello"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	r := Resolver{GitRunner: gr, Dir: dir, DefaultHost: "fallback.sh"}
	got, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Host != "shithub.sh" || got.Owner != "octo" || got.Name != "hello" {
		t.Errorf("got %+v", got)
	}
}

func TestResolveFromGitRemote(t *testing.T) {
	gr, dir := newRepo(t)
	if err := git.AddRemote(gr, dir, "origin", "https://shithub.sh/octo/hello.git"); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	r := Resolver{GitRunner: gr, Dir: dir, DefaultHost: "fallback.sh"}
	got, err := r.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Host != "shithub.sh" || got.Owner != "octo" || got.Name != "hello" {
		t.Errorf("got %+v", got)
	}
}

func TestResolveErrorsWhenUnresolvable(t *testing.T) {
	gr, dir := newRepo(t)
	r := Resolver{GitRunner: gr, Dir: dir, DefaultHost: "shithub.sh"}
	if _, err := r.Resolve(); err == nil {
		t.Fatal("expected error when no source supplies a repo")
	}
}

func TestCloneURL(t *testing.T) {
	ref := RepoRef{Host: "shithub.sh", Owner: "octo", Name: "hello"}
	if got := CloneURL(ref, "https"); got != "https://shithub.sh/octo/hello.git" {
		t.Errorf("https: %q", got)
	}
	if got := CloneURL(ref, "ssh"); got != "git@shithub.sh:octo/hello.git" {
		t.Errorf("ssh: %q", got)
	}
	bare := RepoRef{Owner: "octo", Name: "hello"}
	if got := CloneURL(bare, "https"); !strings.HasPrefix(got, "https://shithub.sh/") {
		t.Errorf("empty host fallback: %q", got)
	}
}

func TestWebURL(t *testing.T) {
	ref := RepoRef{Host: "shithub.sh", Owner: "octo", Name: "hello"}
	if got := WebURL(ref); got != "https://shithub.sh/octo/hello" {
		t.Errorf("WebURL: %q", got)
	}
}

// newRepo wires up a tiny git working tree for tests that need real
// .git/config behavior. Keeps the test file independent of the
// internal/git test suite.
func newRepo(t *testing.T) (git.Runner, string) {
	t.Helper()
	r, err := git.FromPath()
	if err != nil {
		t.Fatalf("FromPath: %v", err)
	}
	dir := t.TempDir()
	if err := git.Init(r, dir, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@e"},
		{"config", "user.name", "t"},
	} {
		if err := r.Run(dir, args, io.Discard, io.Discard); err != nil {
			t.Fatalf("config: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README"},
		{"commit", "-m", "init"},
	} {
		if err := r.Run(dir, args, io.Discard, io.Discard); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	return r, dir
}
