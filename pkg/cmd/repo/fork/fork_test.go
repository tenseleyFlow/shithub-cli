// SPDX-License-Identifier: AGPL-3.0-or-later

package fork

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

// makeBareSource creates a local repo we can clone from in fork tests.
func makeBareSource(t *testing.T) string {
	t.Helper()
	gr, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	src := t.TempDir()
	if err := git.Init(gr, src, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@e"},
		{"config", "user.name", "t"},
	} {
		_ = gr.Run(src, args, io.Discard, io.Discard)
	}
	if err := os.WriteFile(filepath.Join(src, "README"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = gr.Run(src, []string{"add", "."}, io.Discard, io.Discard)
	_ = gr.Run(src, []string{"commit", "-m", "init"}, io.Discard, io.Discard)
	return src
}

func TestForkAPICall(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/octo/hello/forks", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
			DefaultBranch: "trunk", Fork: true,
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "octo/hello",
		RemoteName:  "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("body: %v", err)
	}
	if _, ok := sent["organization"]; ok && sent["organization"] != "" {
		t.Errorf("organization should be empty by default: %v", sent)
	}
}

func TestForkOrgFlag(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/octo/hello/forks", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(repos.Repo{Name: "hello", FullName: "acme/hello", Owner: repos.Owner{Login: "acme"}, Fork: true})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "octo/hello",
		Org:         "acme",
		RemoteName:  "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["organization"] != "acme" {
		t.Errorf("organization not sent: %v", sent)
	}
}

func TestForkCloneWithUpstream(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows TempDir paths look like `C:\...` which dirFromCloneURL
		// in internal/git treats as an SCP-style URL (the colon after the
		// drive letter trips its splitter), so the resolved clone dst
		// doesn't match where git actually puts the clone and the upstream
		// add lands in the wrong dir. The real-world clone URLs the
		// product handles are always http(s)/ssh, so the divergence is
		// test-only — skip here, the Unix paths exercise the same code.
		t.Skip("local-path clone URLs don't round-trip through dirFromCloneURL on Windows")
	}
	tf := cmdutiltest.New(t)
	gr, _ := git.FromPath()
	src := makeBareSource(t)
	parentSrc := makeBareSource(t)

	// Server returns a fork object with parent pointing at parentSrc so
	// the upstream remote is wireable.
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos/octo/hello/forks", 202, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
		DefaultBranch: "trunk", Fork: true,
		CloneURL: src,
		Parent: &repos.Repo{
			Name: "hello", FullName: "octo/hello", Owner: repos.Owner{Login: "octo"},
			CloneURL: parentSrc,
		},
	})

	// Run from a temp working dir so the clone lands somewhere disposable.
	workdir := t.TempDir()
	cwd, _ := os.Getwd()
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		RepoArg:     "octo/hello",
		Clone:       true,
		cloneSet:    true,
		RemoteName:  "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The clone lands in workdir/hello (basename of src), or alongside it.
	// We find any subdir containing .git/upstream remote.
	dirs, _ := os.ReadDir(workdir)
	found := false
	for _, d := range dirs {
		path := filepath.Join(workdir, d.Name())
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			continue
		}
		out, err := gr.Output(path, "config", "--get", "remote.upstream.url")
		if err == nil && strings.TrimSpace(string(out)) == parentSrc {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a cloned subdir with upstream remote pointing at %q", parentSrc)
	}
}

// TestForkWaitsForReadiness covers the audit #144 fix: after the fork
// POST returns, the command polls View(fork) until the server's
// background storage-provisioning job materializes the row. We register
// a View handler that 404s the first two probes and 200s the third —
// then assert the command finished successfully and the probe ran more
// than once.
func TestForkWaitsForReadiness(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos/octo/hello/forks", 202, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
		DefaultBranch: "trunk", Fork: true,
	})
	var probes atomic.Int32
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/me/hello", func(w http.ResponseWriter, _ *http.Request) {
		n := probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not yet"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(repos.Repo{Name: "hello", FullName: "me/hello"})
	})

	opts := &options{
		IO:           tf.IOStreams,
		Prompter:     tf.Prompt,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		GitProtocol:  tf.Factory.GitProtocol,
		RepoArg:      "octo/hello",
		RemoteName:   "origin",
		PollInterval: 1 * time.Millisecond,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := probes.Load(); got != 3 {
		t.Errorf("expected 3 readiness probes (2 x 404 + 1 x 200), got %d", got)
	}
}
