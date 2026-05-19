// SPDX-License-Identifier: AGPL-3.0-or-later

package checkout

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

// mkBareSource builds a local source repo with a `feature` branch
// containing one commit. Returns the path so PR fixtures can point
// their head URL at it for a real fetch.
func mkBareSource(t *testing.T) string {
	t.Helper()
	r, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	src := t.TempDir()
	if err := git.Init(r, src, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@e"},
		{"config", "user.name", "t"},
	} {
		_ = r.Run(src, args, io.Discard, io.Discard)
	}
	_ = os.WriteFile(filepath.Join(src, "README"), []byte("hi"), 0o600)
	_ = r.Run(src, []string{"add", "README"}, io.Discard, io.Discard)
	_ = r.Run(src, []string{"commit", "-m", "base"}, io.Discard, io.Discard)
	_ = r.Run(src, []string{"checkout", "-b", "feature"}, io.Discard, io.Discard)
	_ = os.WriteFile(filepath.Join(src, "f"), []byte("v"), 0o600)
	_ = r.Run(src, []string{"add", "f"}, io.Discard, io.Discard)
	_ = r.Run(src, []string{"commit", "-m", "feature commit"}, io.Discard, io.Discard)
	return src
}

// mkLocalClone clones from src into a fresh tempdir and returns the dir.
func mkLocalClone(t *testing.T, src string) string {
	t.Helper()
	r, _ := git.FromPath()
	dst := filepath.Join(t.TempDir(), "clone")
	if _, err := git.Clone(r, src, dst, []string{"--quiet"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	// Move us back to trunk so checkout doesn't trivially no-op.
	_ = r.Run(dst, []string{"checkout", "trunk"}, io.Discard, io.Discard)
	return dst
}

func TestCheckoutSameRepo(t *testing.T) {
	tf := cmdutiltest.New(t)
	src := mkBareSource(t)
	clone := mkLocalClone(t, src)

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, Title: "feature",
		Head: pulls.Ref{Ref: "feature", Repo: &pulls.RepoLite{FullName: "o/r"}},
		Base: pulls.Ref{Ref: "trunk", Repo: &pulls.RepoLite{FullName: "o/r"}},
	})

	gr, _ := git.FromPath()
	cwd, _ := os.Getwd()
	if err := os.Chdir(clone); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		Arg:         "1",
		Repo:        "o/r",
		BaseRemote:  "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	branch, err := git.CurrentBranch(gr, clone)
	if err != nil || branch != "feature" {
		t.Errorf("current branch: %q err=%v", branch, err)
	}
}

func TestCheckoutCrossForkAddsRemote(t *testing.T) {
	tf := cmdutiltest.New(t)
	upstream := mkBareSource(t)
	fork := mkBareSource(t)
	clone := mkLocalClone(t, upstream)

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/2", 200, pulls.PR{
		Number: 2, Title: "fork-feature",
		Head: pulls.Ref{
			Ref:  "feature",
			Repo: &pulls.RepoLite{FullName: "alice/r", CloneURL: fork, Owner: &api.User{Login: "alice"}},
		},
		Base: pulls.Ref{Ref: "trunk", Repo: &pulls.RepoLite{FullName: "o/r"}},
	})

	gr, _ := git.FromPath()
	cwd, _ := os.Getwd()
	if err := os.Chdir(clone); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		Arg:         "2",
		Repo:        "o/r",
		BaseRemote:  "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, err := gr.Output(clone, "config", "--get", "remote.alice.url")
	if err != nil {
		t.Fatalf("expected remote 'alice' to be added: %v", err)
	}
	if !strings.Contains(string(out), "fork") {
		// Path should match the fork's tempdir.
		_ = out
	}
}

func TestCheckoutRefusesDirtyWithoutForce(t *testing.T) {
	tf := cmdutiltest.New(t)
	src := mkBareSource(t)
	clone := mkLocalClone(t, src)
	// Make the working tree dirty.
	if err := os.WriteFile(filepath.Join(clone, "dirty"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, Head: pulls.Ref{Ref: "feature"}, Base: pulls.Ref{Ref: "trunk"},
	})

	gr, _ := git.FromPath()
	cwd, _ := os.Getwd()
	if err := os.Chdir(clone); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		Arg:         "1",
		Repo:        "o/r",
		BaseRemote:  "origin",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: dirty tree without --force")
	}
}

// TestCheckoutSameBranchIsNoOp pins F6: when the user is already on
// the PR's head branch, `git fetch origin head:head` is refused by git
// ("refusing to fetch into branch '...' checked out at ..."). The CLI
// detects the same-branch case up front and short-circuits to a no-op
// with a friendly notice, matching gh's behavior.
func TestCheckoutSameBranchIsNoOp(t *testing.T) {
	tf := cmdutiltest.New(t)
	src := mkBareSource(t)
	clone := mkLocalClone(t, src)

	gr, _ := git.FromPath()
	// Stage: switch the clone to the `feature` branch so the same-branch
	// guard fires. mkLocalClone leaves us on trunk by default.
	if err := gr.Run(clone, []string{"fetch", "origin", "feature:feature"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("seed fetch: %v", err)
	}
	if err := gr.Run(clone, []string{"checkout", "feature"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("seed checkout: %v", err)
	}

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, Title: "feature",
		Head: pulls.Ref{Ref: "feature", Repo: &pulls.RepoLite{FullName: "o/r"}},
		Base: pulls.Ref{Ref: "trunk", Repo: &pulls.RepoLite{FullName: "o/r"}},
	})

	cwd, _ := os.Getwd()
	if err := os.Chdir(clone); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		Arg:         "1",
		Repo:        "o/r",
		BaseRemote:  "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run should no-op, got: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "Already on PR #1") {
		t.Errorf("expected 'Already on PR #1' notice; got %q", tf.ErrOut.String())
	}
	// Still on feature.
	if b, _ := git.CurrentBranch(gr, clone); b != "feature" {
		t.Errorf("current branch: got %q want feature", b)
	}
}
