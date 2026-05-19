// SPDX-License-Identifier: AGPL-3.0-or-later

package edit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/labels"
)

func TestEditRenameAndColor(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/labels/old", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(labels.Label{Name: "new", Color: "ff0000"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "old",
		Repo:        "o/r",
		NewName:     "new",
		newNameSet:  true,
		Color:       "f00",
		colorSet:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// E6: server expects `name` for renames, not `new_name`.
	if !strings.Contains(string(body), `"name":"new"`) {
		t.Errorf("rename: %s", body)
	}
	if !strings.Contains(string(body), `"color":"ff0000"`) {
		t.Errorf("color expansion: %s", body)
	}
}

func TestEditNoFlagsErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when no edit flags set")
	}
}

func TestEditBadColor(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
		Color:       "purple",
		colorSet:    true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected color validation error")
	}
}
