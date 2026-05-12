// SPDX-License-Identifier: AGPL-3.0-or-later

package sync

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestSyncHappyPath(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/me/hello", 200, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"}, DefaultBranch: "trunk", Fork: true,
	})
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos/me/hello/merge-upstream", 200, repos.MergeUpstreamResult{
		Message: "Successfully fetched and fast-forwarded from upstream", MergeType: "fast-forward", BaseBranch: "trunk",
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "me/hello",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "fast-forward") && !strings.Contains(tf.ErrOut.String(), "Successfully") {
		t.Errorf("expected success message; got %q", tf.ErrOut.String())
	}
}

func TestSyncBranchOverride(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos/me/hello/merge-upstream", 200, repos.MergeUpstreamResult{
		Message: "ok", MergeType: "fast-forward",
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "me/hello",
		Branch:      "feature",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// We sent the explicit branch, so no metadata fetch should be needed.
	for _, c := range tf.Server.Calls() {
		if c.Method == "GET" && strings.HasSuffix(c.Path, "/me/hello") {
			t.Errorf("unexpected metadata fetch when --branch passed")
		}
	}
}

// TestSyncFallsBackToLocalFetchAndMerge exercises the documented
// server+local fallback (audit #145): when the server's merge-upstream
// endpoint returns an error and a git working tree is available, the
// command runs `git fetch upstream && git merge --ff-only` locally and
// reports success.
func TestSyncFallsBackToLocalFetchAndMerge(t *testing.T) {
	gr, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}

	// Build a local upstream with one commit and a fork-clone of it.
	upstream := t.TempDir()
	if err := git.Init(gr, upstream, "trunk"); err != nil {
		t.Fatalf("Init upstream: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@e"},
		{"config", "user.name", "t"},
	} {
		_ = gr.Run(upstream, args, io.Discard, io.Discard)
	}
	if err := os.WriteFile(filepath.Join(upstream, "README"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = gr.Run(upstream, []string{"add", "."}, io.Discard, io.Discard)
	_ = gr.Run(upstream, []string{"commit", "-m", "init"}, io.Discard, io.Discard)

	fork := t.TempDir()
	if err := gr.Run("", []string{"clone", upstream, fork}, io.Discard, io.Discard); err != nil {
		t.Fatalf("clone fork: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@e"},
		{"config", "user.name", "t"},
		{"remote", "add", "upstream", upstream},
	} {
		_ = gr.Run(fork, args, io.Discard, io.Discard)
	}

	// Advance upstream by one commit so the fast-forward has work to do.
	if err := os.WriteFile(filepath.Join(upstream, "NEW"), []byte("y"), 0o600); err != nil {
		t.Fatalf("write upstream NEW: %v", err)
	}
	_ = gr.Run(upstream, []string{"add", "."}, io.Discard, io.Discard)
	_ = gr.Run(upstream, []string{"commit", "-m", "advance"}, io.Discard, io.Discard)

	// Server says no — forcing the fallback.
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/me/hello", 200, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"}, DefaultBranch: "trunk", Fork: true,
	})
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/me/hello/merge-upstream", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"endpoint parked"}`))
	})

	// Run from inside the fork so IsRepo("") detects it.
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(fork); err != nil {
		t.Fatalf("chdir fork: %v", err)
	}

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitRunner:   gr,
		RepoArg:     "me/hello",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "falling back to local") {
		t.Errorf("expected fallback notice on stderr; got %q", tf.ErrOut.String())
	}
	if !strings.Contains(tf.ErrOut.String(), "synced me/hello from upstream/trunk locally") {
		t.Errorf("expected local-sync success line; got %q", tf.ErrOut.String())
	}
	// The fast-forward should have pulled the new file from upstream.
	if _, err := os.Stat(filepath.Join(fork, "NEW")); err != nil {
		t.Errorf("upstream commit not fast-forwarded into fork: %v", err)
	}
}

func TestSyncServerErrorPropagates(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/me/hello", 200, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"}, DefaultBranch: "trunk", Fork: true,
	})
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/me/hello/merge-upstream", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"non-fast-forward"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "me/hello",
		// No GitRunner -> local fallback is skipped, error is surfaced.
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when server rejects merge-upstream and no local fallback available")
	}
}
