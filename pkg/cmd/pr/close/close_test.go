// SPDX-License-Identifier: AGPL-3.0-or-later

package close

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestClosePatchesState(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{Number: 1, State: "open"})

	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.PR{Number: 1, State: "closed"})
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
	if p["state"] != "closed" {
		t.Errorf("state: %v", p)
	}
}

func TestCloseWithComment(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{Number: 1, State: "open"})
	tf.Server.RegisterJSON(http.MethodPatch, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{Number: 1, State: "closed"})
	var seen bool
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		seen = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Comment:     "wontfix",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !seen {
		t.Error("expected comment POST")
	}
}
