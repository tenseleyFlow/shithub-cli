// SPDX-License-Identifier: AGPL-3.0-or-later

package review

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestReviewApprove(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pulls.Review{ID: 1, State: "APPROVED"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Approve:     true,
		Body:        "lgtm",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"event":"APPROVE"`) {
		t.Errorf("event: %s", body)
	}
	if !strings.Contains(string(body), `"body":"lgtm"`) {
		t.Errorf("body: %s", body)
	}
}

func TestReviewRequestChangesNeedsBody(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:             tf.IOStreams,
		Prompter:       tf.Prompt,
		HTTPClient:     tf.Factory.HTTPClient,
		ConfigFn:       tf.Factory.Config,
		DefaultHost:    tf.Factory.DefaultHost,
		Opener:         func(string) error { return nil },
		Arg:            "1",
		Repo:           "o/r",
		RequestChanges: true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: request-changes requires body")
	}
}

func TestReviewMutexFlags(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Approve:     true,
		Comment:     true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected mutex error")
	}
}

func TestReviewWebOpensURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
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
	if !strings.HasSuffix(opened, "/o/r/pull/1/files") {
		t.Errorf("URL: %q", opened)
	}
}

func TestReviewNonTTYNoEventErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: non-TTY needs an event flag")
	}
}

func TestReviewCommentBodyFile(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"state":"COMMENTED"}`))
	})
	tf.In.WriteString("stdin body")

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		ConfigFn:    tf.Factory.Config,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Comment:     true,
		BodyFile:    "-",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"event":"COMMENT"`) {
		t.Errorf("event: %s", body)
	}
	if !strings.Contains(string(body), `"body":"stdin body"`) {
		t.Errorf("body-file=- not read: %s", body)
	}
}
