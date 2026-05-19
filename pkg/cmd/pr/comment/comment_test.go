// SPDX-License-Identifier: AGPL-3.0-or-later

package comment

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

func TestCommentCreate(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issues.Comment{ID: 99, HTMLURL: "https://shithub.sh/o/r/pulls/1#issuecomment-99"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Body:        "ping",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"body":"ping"`) {
		t.Errorf("body: %s", body)
	}
}

func TestCommentEditLastFindsCallersLast(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Comment{
			{ID: 10, Body: "first", User: &api.User{Login: "octo"}},
			{ID: 11, Body: "mine 1", User: &api.User{Login: "mf"}},
			{ID: 13, Body: "mine 2", User: &api.User{Login: "mf"}},
		})
	})
	var seenPath string
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/comments/13", func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Comment{ID: 13})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Body:        "edited",
		EditLast:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(seenPath, "/13") {
		t.Errorf("expected ID 13, hit %q", seenPath)
	}
}

func TestCommentEditLastCreateIfNone(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Comment{
			{ID: 10, Body: "other", User: &api.User{Login: "octo"}},
		})
	})
	var posted bool
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		posted = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":42}`))
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		ConfigFn:     tf.Factory.Config,
		DefaultHost:  tf.Factory.DefaultHost,
		Opener:       func(string) error { return nil },
		Arg:          "1",
		Repo:         "o/r",
		Body:         "first by me",
		EditLast:     true,
		CreateIfNone: true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !posted {
		t.Error("--create-if-none should fall through to POST")
	}
}

func TestCommentEditLastNoCreateErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Comment{})
	})
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Body:        "x",
		EditLast:    true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when no prior comment + no --create-if-none")
	}
}

func TestCommentBodyRequired(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: body required")
	}
}

func TestCommentWebOpensURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "1",
		Repo:        "o/r",
		Web:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/pulls/1") {
		t.Errorf("URL: %q", opened)
	}
}
