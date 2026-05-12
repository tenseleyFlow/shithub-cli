// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestListRendersRuns(t *testing.T) {
	tf := cmdutiltest.New(t)
	now := time.Now()
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/runs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.RunsResponse{
			TotalCount: 1,
			WorkflowRuns: []actions.WorkflowRun{
				{ID: 100, Name: "CI", Status: "completed", Conclusion: "success", HeadBranch: "trunk", Event: "push", UpdatedAt: now},
			},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Limit:       30,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "100") || !strings.Contains(out, "success") {
		t.Errorf("row missing: %q", out)
	}
}

func TestListPassesFiltersOnQuery(t *testing.T) {
	tf := cmdutiltest.New(t)
	var query string
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.RunsResponse{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Branch:      "trunk",
		Event:       "push",
		Status:      "completed",
		Limit:       30,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"branch=trunk", "event=push", "status=completed"} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q: %s", want, query)
		}
	}
}

func TestListWorkflowFlagHitsScopedEndpoint(t *testing.T) {
	tf := cmdutiltest.New(t)
	hit := false
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/workflows/ci.yml/runs", func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.RunsResponse{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Workflow:    "ci.yml",
		Limit:       30,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !hit {
		t.Errorf("--workflow ci.yml did not hit the scoped endpoint")
	}
}

func TestListEmptyMessage(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/runs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.RunsResponse{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Limit:       30,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "no workflow runs found") {
		t.Errorf("empty-state message missing: %q", tf.ErrOut.String())
	}
}
