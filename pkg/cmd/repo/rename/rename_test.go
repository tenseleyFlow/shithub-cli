// SPDX-License-Identifier: AGPL-3.0-or-later

package rename

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestRenamePatchesName(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/old", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "newname", FullName: "o/newname", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk",
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "o/old",
		NewName:     "newname",
		Yes:         true,
		NoRemote:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if got["name"] != "newname" {
		t.Errorf("name not patched: %v", got)
	}
}

// C7: if the server returns 200 with the OLD name (the exact failure
// mode the C-audit observed — `shithub repo rename foo/bar` printing
// "Renamed to <old>" against a no-op response), the CLI must refuse
// to claim success. The defensive check is case-insensitive because
// lifecycle.Rename lowercases the server-side name.
func TestRenameDetectsServerNoopAsError(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/old", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Server lies: 200 but the returned name is unchanged.
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "old", FullName: "o/old", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk",
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "o/old",
		NewName:     "newname",
		Yes:         true,
		NoRemote:    true,
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when server returns the old name; got success")
	}
	if !strings.Contains(err.Error(), "newname") || !strings.Contains(err.Error(), "old") {
		t.Errorf("error message should name both expected and returned name; got %q", err.Error())
	}
}

// C7: case differences between requested and returned name don't count
// as a mismatch (lifecycle.Rename lowercases on the server side).
func TestRenameAcceptsLowercasedServerResponse(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/old", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "newname", FullName: "o/newname", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk",
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "o/old",
		NewName:     "NewName",
		Yes:         true,
		NoRemote:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRenameRejectsSameName(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "o/r",
		NewName:     "r",
		Yes:         true,
		NoRemote:    true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when renaming to same name")
	}
}

// initGitTreeWithOrigin spins up a fresh `git init` working tree and
// sets origin to the supplied URL. Returns the dir + a function to
// chdir back. Test must call the returned cleanup.
func initGitTreeWithOrigin(t *testing.T, originURL string) (dir string, restore func()) {
	t.Helper()
	gr, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	dir = t.TempDir()
	if err := git.Init(gr, dir, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := git.AddRemote(gr, dir, "origin", originURL); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	return dir, func() { _ = os.Chdir(old) }
}

// TestRenameDoesNotClobberUnrelatedOrigin is the regression test for
// CX2: previously, `repo rename` overwrote the CWD's `origin` even
// when that origin pointed at a completely different repo. Run inside
// a git tree whose origin is `example.com:other/unrelated.git` and
// confirm origin survives the rename of `o/old -> o/newname`.
func TestRenameDoesNotClobberUnrelatedOrigin(t *testing.T) {
	originalURL := "git@example.com:other/unrelated.git"
	dir, restore := initGitTreeWithOrigin(t, originalURL)
	defer restore()

	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/old", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "newname", FullName: "o/newname", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk",
		})
	})
	gr, _ := git.FromPath()
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		RepoArg:     "o/old",
		NewName:     "newname",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// origin URL must be unchanged.
	got, err := git.ResolveRemote(dir, "origin")
	if err != nil {
		t.Fatalf("ResolveRemote: %v", err)
	}
	if got.Owner != "other" || got.Repo != "unrelated" {
		t.Errorf("origin was clobbered: got %s, want other/unrelated", got.String())
	}

	// And the user must see why we declined to rotate.
	stderr := tf.ErrOut.String()
	if !strings.Contains(stderr, "origin not updated") {
		t.Errorf("expected 'origin not updated' notice; got: %s", stderr)
	}
}

// TestRenameRotatesMatchingOrigin: the happy path we still want —
// inside a clone of the repo being renamed, origin should rotate to
// the new URL.
func TestRenameRotatesMatchingOrigin(t *testing.T) {
	dir, restore := initGitTreeWithOrigin(t, "https://shithub.test/o/old.git")
	defer restore()

	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/old", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "newname", FullName: "o/newname", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk",
			CloneURL: "https://shithub.test/o/newname.git",
		})
	})
	gr, _ := git.FromPath()
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		RepoArg:     "o/old",
		NewName:     "newname",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := git.ResolveRemote(dir, "origin")
	if err != nil {
		t.Fatalf("ResolveRemote: %v", err)
	}
	if got.Owner != "o" || got.Repo != "newname" {
		t.Errorf("origin did not rotate: got %s, want o/newname", got.String())
	}
	if !strings.Contains(tf.ErrOut.String(), "Updated origin") {
		t.Errorf("expected 'Updated origin' line; got: %s", tf.ErrOut.String())
	}
}
