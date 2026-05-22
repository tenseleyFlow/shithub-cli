// SPDX-License-Identifier: AGPL-3.0-or-later

package ready

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

func TestReadyFlipsDraftFalse(t *testing.T) {
	tf := cmdutiltest.New(t)
	// H5: ready does a GET first to pre-flight state, so the test must
	// stage a draft PR for the View call.
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Draft: true,
	})
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.PR{Number: 1})
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
	_ = json.Unmarshal(body, &p)
	if p["draft"] != false {
		t.Errorf("draft: %v", p)
	}
}

// TestReadyOnMergedPRRefuses pins H5: pre-fix the CLI lied with
// "Marked PR as ready" after the server silently accepted the no-op
// PATCH on a merged PR. Now it surfaces the merged state up front.
func TestReadyOnMergedPRRefuses(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/3", 200, pulls.PR{
		Number: 3, State: "closed", Merged: true,
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "3",
		Repo:        "o/r",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on merged PR")
	}
	if !strings.Contains(err.Error(), "already merged") {
		t.Errorf("error should mention merged: %v", err)
	}
}

// TestReadyOnReadyPRRefuses pins H5: same lie shape, ready PR (not
// draft). Now refused with a clear message.
func TestReadyOnReadyPRRefuses(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/7", 200, pulls.PR{
		Number: 7, State: "open", Draft: false,
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "7",
		Repo:        "o/r",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when PR is already ready")
	}
	if !strings.Contains(err.Error(), "already marked as ready") {
		t.Errorf("error should mention already ready: %v", err)
	}
}

// TestReadyUndoRejectedClientSide pins F5 / F23: until the server adds
// ready→draft (POST /pulls/{n} returns 422 today), the CLI pre-flight
// rejects --undo locally with a single clean line. Pre-fix the user
// saw `shithub: shithub API: 422 ready→draft is not supported` after a
// pointless roundtrip.
func TestReadyUndoRejectedClientSide(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Undo:        true,
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected --undo to fail client-side")
	}
	if !strings.Contains(err.Error(), "not yet supported") {
		t.Errorf("error should mention support state: %v", err)
	}
	// No PATCH should have hit the fake server — the guard runs before
	// the HTTP call.
	for _, c := range tf.Server.Calls() {
		if c.Method == http.MethodPatch {
			t.Errorf("unexpected PATCH call: %+v", c)
		}
	}
}

// TestReadyWrongNamespaceRedirects pins H2: `pr ready <issue-number>`
// surfaces a friendly redirect rather than "pull request not found".
func TestReadyWrongNamespaceRedirects(t *testing.T) {
	tf := cmdutiltest.New(t)
	// #5 is an issue; the pulls endpoint would 404.
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues/5", 200, issues.Issue{Number: 5})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "5",
		Repo:        "o/r",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("want cross-namespace error, got nil")
	}
	if !strings.Contains(err.Error(), "is an issue") {
		t.Errorf("error should redirect: %v", err)
	}
	if !strings.Contains(err.Error(), "shithub issue ready 5") {
		t.Errorf("error should suggest issue ready: %v", err)
	}
}
