// SPDX-License-Identifier: AGPL-3.0-or-later

package code

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
)

func TestCodeQualifiersAndSnippet(t *testing.T) {
	tf := cmdutiltest.New(t)
	var gotQ string
	tf.Server.Handle(http.MethodGet, "/api/v1/search/code", func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[search.CodeItem]{
			TotalCount: 1,
			Items: []search.CodeItem{{
				Name: "README.md", Path: "README.md",
				TextMatches: []search.TextMatch{{Fragment: "TODO: something\nimportant"}},
			}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "TODO",
		Filename:    "README.md",
		Size:        ">100",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"TODO", "filename:README.md", "size:>=101"} {
		if !strings.Contains(gotQ, want) {
			t.Errorf("q = %q; missing %q", gotQ, want)
		}
	}
	if !strings.Contains(tf.Out.String(), "TODO:") {
		t.Errorf("missing snippet: %q", tf.Out.String())
	}
}
