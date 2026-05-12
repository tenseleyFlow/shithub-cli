// SPDX-License-Identifier: AGPL-3.0-or-later

package rename

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestRenamePatchesName(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/old", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "newname", FullName: "o/newname", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk",
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "o/old",
		NewName:     "newname",
		Yes:         true,
		NoRemote:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if got["name"] != "newname" {
		t.Errorf("name not patched: %v", got)
	}
}

func TestRenameRejectsSameName(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		GitProtocol: tf.Factory.GitProtocol,
		RepoArg:     "o/r",
		NewName:     "r",
		Yes:         true,
		NoRemote:    true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when renaming to same name")
	}
}
