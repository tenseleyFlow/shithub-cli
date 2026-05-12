// SPDX-License-Identifier: AGPL-3.0-or-later

package archive

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestArchiveSendsArchivedTrue(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{Name: "r", FullName: "o/r", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
		Yes:         true,
		archive:     true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if got["archived"] != true {
		t.Errorf("archived flag not set: %v", got)
	}
}

func TestUnarchiveSendsArchivedFalse(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{Name: "r", FullName: "o/r", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
		Yes:         true,
		archive:     false,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if got["archived"] != false {
		t.Errorf("expected archived=false; got %v", got)
	}
}
