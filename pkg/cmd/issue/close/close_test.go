// SPDX-License-Identifier: AGPL-3.0-or-later

package close

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestCloseWithReasonAndComment(t *testing.T) {
	tf := cmdutiltest.New(t)
	var patchBody, commentBody json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		commentBody = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(issues.Comment{ID: 5})
	})
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		patchBody = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1, State: "closed"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Reason:      "not_planned",
		Comment:     "wontfix",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var c map[string]any
	_ = json.Unmarshal(commentBody, &c)
	if c["body"] != "wontfix" {
		t.Errorf("comment body: %v", c)
	}
	var p map[string]any
	_ = json.Unmarshal(patchBody, &p)
	if p["state"] != "closed" {
		t.Errorf("state: %v", p)
	}
	if p["state_reason"] != "not_planned" {
		t.Errorf("state_reason: %v", p)
	}
}

func TestCloseInvalidReason(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Reason:      "bogus",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error on invalid reason")
	}
}

func TestCloseWithoutReason(t *testing.T) {
	tf := cmdutiltest.New(t)
	var patchBody json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		patchBody = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1, State: "closed"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var p map[string]any
	_ = json.Unmarshal(patchBody, &p)
	if _, ok := p["state_reason"]; ok {
		t.Errorf("state_reason should be omitted when not set: %v", p)
	}
}

// TestCloseWrongNamespaceRedirects pins H2: `issue close <PR-number>`
// surfaces a friendly redirect rather than the server's raw 422 about
// the shared issue+PR table. Pre-fix the user saw
// "shithub: shithub API: 422 title, body, state, ... must be edited
// via PATCH /pulls/{N}".
func TestCloseWrongNamespaceRedirects(t *testing.T) {
	tf := cmdutiltest.New(t)
	// #3 is a PR; the issue endpoint would 404 it.
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/3", 200, pulls.PR{Number: 3})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "3",
		Repo:        "o/r",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("want cross-namespace error, got nil")
	}
	if !strings.Contains(err.Error(), "is a pull request") {
		t.Errorf("error should redirect: %v", err)
	}
	if !strings.Contains(err.Error(), "shithub pr close 3") {
		t.Errorf("error should suggest pr close: %v", err)
	}
	// Crucially, NO PATCH should have hit the issues endpoint.
	for _, c := range tf.Server.Calls() {
		if c.Method == http.MethodPatch {
			t.Errorf("unexpected PATCH against issues endpoint: %+v", c)
		}
	}
}
