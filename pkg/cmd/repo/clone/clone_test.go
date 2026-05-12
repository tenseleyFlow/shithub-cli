// SPDX-License-Identifier: AGPL-3.0-or-later

package clone

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

// makeBareSource creates a bare-style local repo we can clone from. Returns
// the path to it.
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

func TestCloneFromOwnerRepoForm(t *testing.T) {
	tf := cmdutiltest.New(t)
	gr, _ := git.FromPath()
	src := makeBareSource(t)

	// Server returns metadata; clone URL points at the local src so
	// the actual git invocation succeeds.
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/octo/hello", 200, repos.Repo{
		Name: "hello", FullName: "octo/hello", Owner: repos.Owner{Login: "octo"},
		DefaultBranch: "trunk", CloneURL: src,
	})

	dst := filepath.Join(t.TempDir(), "out")
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		Target:      "octo/hello",
		Dir:         dst,
		UpstreamRem: "upstream",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		t.Errorf(".git missing: %v", err)
	}
}

func TestCloneForkAddsUpstream(t *testing.T) {
	tf := cmdutiltest.New(t)
	gr, _ := git.FromPath()
	src := makeBareSource(t)

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/me/hello", 200, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
		DefaultBranch: "trunk", CloneURL: src, Fork: true,
		Parent: &repos.Repo{
			FullName: "octo/hello", Owner: repos.Owner{Login: "octo"},
			CloneURL: "https://shithub.sh/octo/hello.git",
		},
	})

	dst := filepath.Join(t.TempDir(), "out")
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		Target:      "me/hello",
		Dir:         dst,
		UpstreamRem: "upstream",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := gr.Output(dst, "config", "--get", "remote.upstream.url")
	if err != nil {
		t.Fatalf("upstream not added: %v", err)
	}
	if !strings.Contains(string(got), "octo/hello") {
		t.Errorf("upstream url unexpected: %q", got)
	}
}

func TestCloneForkSkipsUpstreamWhenFlagSet(t *testing.T) {
	tf := cmdutiltest.New(t)
	gr, _ := git.FromPath()
	src := makeBareSource(t)

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/me/hello", 200, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
		DefaultBranch: "trunk", CloneURL: src, Fork: true,
		Parent: &repos.Repo{FullName: "octo/hello", CloneURL: "https://shithub.sh/octo/hello.git"},
	})

	dst := filepath.Join(t.TempDir(), "out")
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		Target:      "me/hello",
		Dir:         dst,
		NoUpstream:  true,
		UpstreamRem: "upstream",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := gr.Output(dst, "config", "--get", "remote.upstream.url"); err == nil {
		t.Error("--no-upstream-remote should have prevented upstream remote")
	}
}

func TestCloneFromURLSkipsMetadataFetch(t *testing.T) {
	tf := cmdutiltest.New(t)
	fr := &fakeRunner{}

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   fr,
		Target:      "https://shithub.sh/octo/hello.git",
		Dir:         "/tmp/never-actually-cloned",
		UpstreamRem: "upstream",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(tf.Server.Calls()) != 0 {
		t.Errorf("URL form should skip metadata fetch; calls=%v", tf.Server.Calls())
	}
	if len(fr.runs) == 0 || fr.runs[0][0] != "clone" {
		t.Errorf("expected clone invocation, got %v", fr.runs)
	}
}

// fakeRunner records git invocations without shelling out. Used by the
// URL-form clone test to keep the assertion focused on routing logic
// rather than git's success.
type fakeRunner struct {
	runs [][]string
}

func (f *fakeRunner) Run(_ string, args []string, _, _ io.Writer) error {
	f.runs = append(f.runs, args)
	return nil
}

func (f *fakeRunner) Output(_ string, args ...string) ([]byte, error) {
	f.runs = append(f.runs, args)
	return nil, nil
}
