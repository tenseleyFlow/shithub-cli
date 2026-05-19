// SPDX-License-Identifier: AGPL-3.0-or-later

package create

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

// mkRepoOnBranch sets up a local repo with a `trunk` base, switches to
// `feature`, and commits the given subjects. Returns runner + dir.
func mkRepoOnBranch(t *testing.T, subjects ...string) (git.Runner, string) {
	t.Helper()
	r, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	dir := t.TempDir()
	if err := git.Init(r, dir, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@e"},
		{"config", "user.name", "t"},
	} {
		_ = r.Run(dir, args, io.Discard, io.Discard)
	}
	if err := os.WriteFile(filepath.Join(dir, "BASE"), []byte("base"), 0o600); err != nil {
		t.Fatalf("base write: %v", err)
	}
	_ = r.Run(dir, []string{"add", "BASE"}, io.Discard, io.Discard)
	_ = r.Run(dir, []string{"commit", "-m", "base"}, io.Discard, io.Discard)
	_ = r.Run(dir, []string{"checkout", "-b", "feature"}, io.Discard, io.Discard)
	for i, s := range subjects {
		name := "f" + string(rune('a'+i))
		_ = os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600)
		_ = r.Run(dir, []string{"add", name}, io.Discard, io.Discard)
		_ = r.Run(dir, []string{"commit", "-m", s}, io.Discard, io.Discard)
	}
	return r, dir
}

func TestCreateFlagDriven(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{DefaultBranch: "trunk"})

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pulls.PR{Number: 7, HTMLURL: "https://shithub.sh/o/r/pulls/7"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Title:       "hello",
		Body:        "world",
		Head:        "feature",
		Draft:       true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["title"] != "hello" || sent["draft"] != true {
		t.Errorf("body: %v", sent)
	}
	if sent["head"] != "feature" || sent["base"] != "trunk" {
		t.Errorf("head/base: %v", sent)
	}
}

func TestCreateFillReadsCommits(t *testing.T) {
	tf := cmdutiltest.New(t)
	gr, dir := mkRepoOnBranch(t, "first subject\n\nfirst body", "second subject", "third subject")

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{DefaultBranch: "trunk"})
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pulls.PR{Number: 1, HTMLURL: "https://shithub.sh/o/r/pulls/1"})
	})

	// Run inside dir so CurrentBranch + Fill find the right commits.
	cwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		GitRunner:   gr,
		Repo:        "o/r",
		Fill:        true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["title"] != "first subject" {
		t.Errorf("title from fill: %v", sent)
	}
	if !strings.Contains(sent["body"].(string), "second subject") {
		t.Errorf("body from fill: %v", sent)
	}
}

func TestCreateWebSkipsAPI(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{DefaultBranch: "trunk"})

	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Title:       "subj",
		Body:        "body",
		Head:        "feature",
		Web:         true,
		Draft:       true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(opened, "compare/trunk...feature") {
		t.Errorf("compare URL: %q", opened)
	}
	if !strings.Contains(opened, "draft=1") {
		t.Errorf("draft flag missing: %q", opened)
	}
	// API call to /repos/o/r is allowed (to read default branch); POST should not have happened.
	for _, c := range tf.Server.Calls() {
		if c.Method == "POST" {
			t.Errorf("--web should skip POST: %v", c)
		}
	}
}

func TestCreateMutuallyExclusiveFillFlags(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Fill:        true,
		FillFirst:   true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected mutex error")
	}
}

func TestCreateTitleRequiredWhenNoTTY(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{DefaultBranch: "trunk"})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Head:        "feature",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: title required in non-TTY mode without --fill")
	}
}

func TestCreateExplicitBaseOverridesDefault(t *testing.T) {
	tf := cmdutiltest.New(t)
	// Default-branch lookup should NOT fire when --base is set.
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("default-branch lookup should be skipped when --base provided")
	})

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pulls.PR{Number: 1})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Title:       "t",
		Head:        "feature",
		Base:        "release",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["base"] != "release" {
		t.Errorf("base: %v", sent)
	}
}
