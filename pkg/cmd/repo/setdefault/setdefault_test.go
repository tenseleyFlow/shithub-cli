// SPDX-License-Identifier: AGPL-3.0-or-later

package setdefault

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
)

// chdirTo points the runner at a fresh tempdir-based repo so the .git/config
// writes don't pollute the actual repo. Returns the original cwd to restore.
func chdirToRepo(t *testing.T) (string, git.Runner) {
	t.Helper()
	gr, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	cwd, _ := os.Getwd()
	dir := t.TempDir()
	if err := git.Init(gr, dir, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	return dir, gr
}

func TestSetDefaultWrites(t *testing.T) {
	tf := cmdutiltest.New(t)
	dir, gr := chdirToRepo(t)
	_ = dir

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitRunner:   gr,
		RepoArg:     "octo/hello",
		Hostname:    "shithub.sh",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, _ := gr.Output("", "config", "--get", ConfigKey)
	if strings.TrimSpace(string(out)) != "shithub.sh:octo/hello" {
		t.Errorf("config value: %q", out)
	}
}

func TestSetDefaultUnsets(t *testing.T) {
	tf := cmdutiltest.New(t)
	_, gr := chdirToRepo(t)
	// Seed a value.
	_ = gr.Run("", []string{"config", ConfigKey, "shithub.sh:octo/hello"}, io.Discard, io.Discard)

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitRunner:   gr,
		Unset:       true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out, _ := gr.Output("", "config", "--get", ConfigKey)
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("expected empty after unset; got %q", out)
	}
}

func TestSetDefaultRequiresArgOrUnset(t *testing.T) {
	tf := cmdutiltest.New(t)
	_, gr := chdirToRepo(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitRunner:   gr,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when neither arg nor --unset")
	}
}
