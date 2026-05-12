// SPDX-License-Identifier: AGPL-3.0-or-later

package diff

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestDiffPrintsRaw(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/o/r/pulls/1.diff", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("diff --git a/foo b/foo\n@@ -1 +1 @@\n-old\n+new\n"))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Color:       "never",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "diff --git") {
		t.Errorf("diff missing: %q", tf.Out.String())
	}
}

func TestDiffNameOnly(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/1/files", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]pulls.File{
			{Filename: "a.go", Status: "modified"},
			{Filename: "b.md", Status: "added"},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		NameOnly:    true,
		Color:       "never",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "a.go") || !strings.Contains(out, "b.md") {
		t.Errorf("name-only output: %q", out)
	}
}
