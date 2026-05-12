// SPDX-License-Identifier: AGPL-3.0-or-later

package git

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRepo initializes a git working tree under t.TempDir with one
// committed file so other tests can assume CurrentBranch/IsClean work.
func newTestRepo(t *testing.T, defaultBranch string) (Runner, string) {
	t.Helper()
	r, err := FromPath()
	if err != nil {
		t.Fatalf("FromPath: %v", err)
	}
	dir := t.TempDir()
	if err := Init(r, dir, defaultBranch); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Local identity so commit succeeds in CI/sandbox environments.
	for _, args := range [][]string{
		{"config", "user.email", "test@shithub.local"},
		{"config", "user.name", "Test"},
	} {
		if err := r.Run(dir, args, io.Discard, io.Discard); err != nil {
			t.Fatalf("config: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README"},
		{"commit", "-m", "init"},
	} {
		var stderr bytes.Buffer
		if err := r.Run(dir, args, io.Discard, &stderr); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stderr.String())
		}
	}
	return r, dir
}

func TestInitCreatesTrunk(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	branch, err := CurrentBranch(r, dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "trunk" {
		t.Errorf("expected default branch 'trunk', got %q", branch)
	}
}

func TestIsRepoYesNo(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	yes, err := IsRepo(r, dir)
	if err != nil || !yes {
		t.Fatalf("IsRepo dir: yes=%v err=%v", yes, err)
	}
	plain := t.TempDir()
	no, err := IsRepo(r, plain)
	if err != nil {
		t.Fatalf("IsRepo non-repo: %v", err)
	}
	if no {
		t.Error("non-repo dir reported as repo")
	}
}

func TestIsCleanFlipsOnUntracked(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	clean, err := IsClean(r, dir)
	if err != nil || !clean {
		t.Fatalf("expected clean after init: %v %v", clean, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scratch"), []byte("y"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	clean, err = IsClean(r, dir)
	if err != nil {
		t.Fatalf("IsClean: %v", err)
	}
	if clean {
		t.Error("expected dirty after writing untracked file")
	}
}

func TestRemoteLifecycle(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	url := "https://shithub.sh/octo/hello.git"

	if has, _ := RemoteExists(r, dir, "origin"); has {
		t.Fatal("origin should not exist yet")
	}

	if err := AddRemote(r, dir, "origin", url); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	if has, _ := RemoteExists(r, dir, "origin"); !has {
		t.Fatal("origin should exist after add")
	}

	if err := RenameRemote(r, dir, "origin", "upstream"); err != nil {
		t.Fatalf("RenameRemote: %v", err)
	}
	if has, _ := RemoteExists(r, dir, "upstream"); !has {
		t.Fatal("upstream should exist after rename")
	}

	newURL := "https://shithub.sh/octo/hello2.git"
	if err := SetRemoteURL(r, dir, "upstream", newURL); err != nil {
		t.Fatalf("SetRemoteURL: %v", err)
	}
	got, err := r.Output(dir, "remote", "get-url", "upstream")
	if err != nil {
		t.Fatalf("get-url: %v", err)
	}
	if strings.TrimSpace(string(got)) != newURL {
		t.Errorf("set-url: want %q got %q", newURL, got)
	}
}

func TestSetGetConfig(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	if err := SetConfig(r, dir, "shithub.default-repo", "octo/hello"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	got, err := GetConfig(r, dir, "shithub.default-repo")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got != "octo/hello" {
		t.Errorf("GetConfig: got %q", got)
	}

	if err := UnsetConfig(r, dir, "shithub.default-repo"); err != nil {
		t.Fatalf("UnsetConfig: %v", err)
	}
	got, _ = GetConfig(r, dir, "shithub.default-repo")
	if got != "" {
		t.Errorf("after unset, GetConfig: %q", got)
	}

	// Unsetting again should be a no-op (idempotent).
	if err := UnsetConfig(r, dir, "shithub.default-repo"); err != nil {
		t.Errorf("UnsetConfig idempotent: %v", err)
	}
}

func TestDirFromCloneURL(t *testing.T) {
	cases := map[string]string{
		"https://shithub.sh/octo/hello.git":  "hello",
		"https://shithub.sh/octo/hello":      "hello",
		"git@shithub.sh:octo/hello.git":      "hello",
		"ssh://git@shithub.sh:22/octo/hello": "hello",
		"https://shithub.sh/octo/hello.git/": "hello",
	}
	for in, want := range cases {
		if got := dirFromCloneURL(in); got != want {
			t.Errorf("dirFromCloneURL(%q): got %q want %q", in, got, want)
		}
	}
}

func TestCloneIntoTempDir(t *testing.T) {
	// Source repo to clone from (local path is a valid git remote).
	r, src := newTestRepo(t, "trunk")
	dst := filepath.Join(t.TempDir(), "clone-dest")

	var out bytes.Buffer
	resolved, err := Clone(r, src, dst, []string{"--quiet"}, &out, &out)
	if err != nil {
		t.Fatalf("Clone: %v\n%s", err, out.String())
	}
	if resolved != dst {
		t.Errorf("resolved dir: got %q want %q", resolved, dst)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		t.Errorf("cloned dir missing .git: %v", err)
	}
}
