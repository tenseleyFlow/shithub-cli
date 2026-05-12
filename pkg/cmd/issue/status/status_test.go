// SPDX-License-Identifier: AGPL-3.0-or-later

package status

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

func TestStatusRendersThreeSections(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("filter") {
		case "mentioned":
			_ = json.NewEncoder(w).Encode([]issues.Issue{
				{Number: 7, Title: "mention me", Repository: &issues.RepoRef{FullName: "x/y"}},
			})
		case "created":
			_ = json.NewEncoder(w).Encode([]issues.Issue{
				{Number: 8, Title: "wrote this", Repository: &issues.RepoRef{FullName: "x/y"}},
			})
		default:
			_ = json.NewEncoder(w).Encode([]issues.Issue{
				{Number: 1, Title: "assigned to me", Repository: &issues.RepoRef{FullName: "o/r"}},
			})
		}
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{
		"Issues assigned to you",
		"Issues mentioning you",
		"Issues opened by you",
		"assigned to me",
		"mention me",
		"wrote this",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestStatusJSONShape(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Issue{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	opts.Exporter.JSONFields = "assigned,mentioned,authored"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(tf.Out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, tf.Out.String())
	}
	for _, k := range []string{"assigned", "mentioned", "authored"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q in %v", k, got)
		}
	}
}
