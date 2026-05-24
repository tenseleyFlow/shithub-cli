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

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

func TestCreateFlagDriven(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 7, HTMLURL: "https://shithub.sh/o/r/issues/7"})
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
		Labels:      []string{"bug,ux"},
		Assignees:   []string{"@me"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["title"] != "hello" {
		t.Errorf("title: %v", sent)
	}
	labels, _ := sent["labels"].([]any)
	if len(labels) != 2 {
		t.Errorf("labels: %v", labels)
	}
	assignees, _ := sent["assignees"].([]any)
	if len(assignees) != 1 || assignees[0] != "mf" {
		t.Errorf("@me did not expand to mf: %v", assignees)
	}
	if !strings.Contains(tf.Out.String(), "https://shithub.sh/o/r/issues/7") {
		t.Errorf("missing URL on stdout: %q", tf.Out.String())
	}
}

func TestCreateWebSkipsAPI(t *testing.T) {
	tf := cmdutiltest.New(t)
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
		Web:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(opened, "/o/r/issues/new") {
		t.Errorf("URL wrong: %q", opened)
	}
	if !strings.Contains(opened, "title=subj") || !strings.Contains(opened, "body=body") {
		t.Errorf("prefilled params missing: %q", opened)
	}
	if len(tf.Server.Calls()) != 0 {
		t.Errorf("--web should skip API; got %d calls", len(tf.Server.Calls()))
	}
}

// TestCreateRejectsMultilineTitle pins audit-I52 at the issue-create
// boundary. The auditor passed `--title $'one\ntwo'` and the CLI
// happily forwarded it; ValidateTitle now intercepts before round-trip.
func TestCreateRejectsMultilineTitle(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Title:       "line one\nline two",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for newline in --title, got nil")
	}
	if !strings.Contains(err.Error(), "single line") {
		t.Errorf("error %q does not mention single-line constraint", err.Error())
	}
	if len(tf.Server.Calls()) != 0 {
		t.Errorf("server should not be called when validation fails; got %d calls", len(tf.Server.Calls()))
	}
}

func TestCreateMissingTitleErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: title required without --title in non-TTY mode")
	}
}

func TestCreateBodyFileWins(t *testing.T) {
	tf := cmdutiltest.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "body.md")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Title:       "subj",
		Body:        "from flag",
		BodyFile:    path,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["body"] != "from file" {
		t.Errorf("body-file should win: %v", sent)
	}
}

func TestCreateMilestoneOmittedWhenZero(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Title:       "subj",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if _, ok := sent["milestone"]; ok {
		t.Errorf("milestone should be omitted when 0: %v", sent)
	}
}
