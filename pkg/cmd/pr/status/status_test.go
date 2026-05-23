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

// prMarker is the empty struct{} pointer signaling "this issue is a PR".
// It's not exported by the issues package; we use a literal in tests.
func prMarker() *struct{} { v := struct{}{}; return &v }

func TestStatusFiltersToPRs(t *testing.T) {
	tf := cmdutiltest.New(t)
	// F29: ListAcrossRepos now routes via /search/issues, branching on
	// the `q` qualifier (`author:@me` / `assignee:@me`). The mentioned
	// scope is deferred — no FTS mention index yet — and returns nil
	// without a round-trip.
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("q")
		var items []issues.Issue
		switch {
		case strings.Contains(q, "author:@me"):
			items = []issues.Issue{
				{Number: 9, Title: "wrote-pr", PullRequest: prMarker(), Repository: &issues.RepoRef{FullName: "x/y"}},
			}
		case strings.Contains(q, "assignee:@me"):
			items = []issues.Issue{
				{Number: 10, Title: "rev-req-pr", PullRequest: prMarker(), Repository: &issues.RepoRef{FullName: "x/y"}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
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
	for _, want := range []string{"Created by you", "Requesting a code review", "wrote-pr", "rev-req-pr"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestStatusJSON(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []issues.Issue{}})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	opts.Exporter.JSONFields = "createdBy,mentioned,reviewRequested"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(tf.Out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, tf.Out.String())
	}
	for _, k := range []string{"createdBy", "mentioned", "reviewRequested"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
}
