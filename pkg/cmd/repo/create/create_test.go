// SPDX-License-Identifier: AGPL-3.0-or-later

package create

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestCreateUserFlagDriven(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/user/repos", func(w http.ResponseWriter, r *http.Request) {
		b, _ := readBody(r)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
			DefaultBranch: "trunk", HTMLURL: "https://shithub.sh/me/hello",
			CloneURL: "https://shithub.sh/me/hello.git",
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		NameArg:     "hello",
		Description: "say hi",
		Private:     true,
		Remote:      "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("body: %v", err)
	}
	if sent["name"] != "hello" || sent["description"] != "say hi" {
		t.Errorf("unexpected POST: %v", sent)
	}
	if sent["visibility"] != "private" {
		t.Errorf("visibility: %v", sent)
	}
}

func TestCreateOrgPositional(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/orgs/acme/repos", 201, repos.Repo{
		Name: "thing", FullName: "acme/thing", Owner: repos.Owner{Login: "acme"},
		DefaultBranch: "trunk", HTMLURL: "https://shithub.sh/acme/thing",
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		NameArg:     "acme/thing",
		Public:      true,
		Remote:      "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodPost, "/api/v1/orgs/acme/repos")
}

func TestCreateTemplate(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos/skeleton/proj/generate", 201, repos.Repo{
		Name: "new", FullName: "me/new", Owner: repos.Owner{Login: "me"},
		DefaultBranch: "trunk", HTMLURL: "https://shithub.sh/me/new",
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		NameArg:     "new",
		Template:    "skeleton/proj",
		Private:     true,
		Remote:      "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodPost, "/api/v1/repos/skeleton/proj/generate")
}

func TestCreateVisibilityMutex(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		NameArg:     "x",
		Public:      true,
		Private:     true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected mutex error")
	}
}

func TestCreatePushRequiresSource(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		NameArg:     "x",
		Public:      true,
		Push:        true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: --push requires --source")
	}
}

func TestCreateInteractive(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.IOStreams.SetStdoutTTY(true)
	tf.Prompt.QueueInput("hello", "say hi")
	tf.Prompt.QueueSelect(1) // Private
	tf.Prompt.QueueConfirm(false, false)

	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/user/repos", 201, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
		DefaultBranch: "trunk", HTMLURL: "https://shithub.sh/me/hello",
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		Remote:      "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCreateSourceWiresRemote(t *testing.T) {
	tf := cmdutiltest.New(t)
	gr, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	src := t.TempDir()
	if err := git.Init(gr, src, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}

	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/user/repos", 201, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"},
		DefaultBranch: "trunk", HTMLURL: "https://shithub.sh/me/hello",
		CloneURL: "https://shithub.sh/me/hello.git",
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		GitRunner:   gr,
		NameArg:     "hello",
		Public:      true,
		Source:      src,
		Remote:      "origin",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Confirm the remote landed.
	got, err := gr.Output(src, "config", "--get", "remote.origin.url")
	if err != nil {
		t.Fatalf("get remote: %v", err)
	}
	want := "https://shithub.sh/me/hello.git"
	if strings.TrimSpace(string(got)) != want {
		t.Errorf("remote url: want %q got %q", want, got)
	}
	// And the source dir exists (sanity check).
	if _, err := statFn(filepath.Join(src, ".git")); err != nil {
		t.Errorf(".git missing: %v", err)
	}
}

func readBody(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 256)
	for {
		n, err := r.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
