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
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestReadyFlipsDraftFalse(t *testing.T) {
	tf := cmdutiltest.New(t)
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
