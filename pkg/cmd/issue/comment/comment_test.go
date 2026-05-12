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
		_ = json.NewEncoder(w).Encode(issues.Comment{ID: 99, HTMLURL: "https://shithub.sh/o/r/issues/1#issuecomment-99"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Body:        "hello",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var c map[string]any
	_ = json.Unmarshal(body, &c)
	if c["body"] != "hello" {
		t.Errorf("body: %v", c)
	}
	if !strings.Contains(tf.Out.String(), "issuecomment-99") {
		t.Errorf("URL missing: %q", tf.Out.String())
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
			{ID: 12, Body: "noise", User: &api.User{Login: "other"}},
			{ID: 13, Body: "mine 2", User: &api.User{Login: "mf"}},
		})
	})
	var seenID string
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/comments/13", func(w http.ResponseWriter, r *http.Request) {
		seenID = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Comment{ID: 13, Body: "edited"})
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
	if !strings.HasSuffix(seenID, "/13") {
		t.Errorf("expected last caller comment ID 13, hit %q", seenID)
	}
}

func TestCommentEditLastErrorsWhenNotCommented(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Comment{
			{ID: 10, Body: "first", User: &api.User{Login: "octo"}},
		})
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
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when caller has no prior comment")
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
