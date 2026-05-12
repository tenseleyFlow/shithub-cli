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

func TestListRendersActiveWorkflows(t *testing.T) {
	tf := cmdutiltest.New(t)
	now := time.Now()
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.WorkflowsResponse{
			TotalCount: 2,
			Workflows: []actions.Workflow{
				{ID: 1, Name: "CI", Path: ".github/workflows/ci.yml", State: "active", UpdatedAt: now},
				{ID: 2, Name: "Old", Path: ".github/workflows/old.yml", State: "disabled_manually", UpdatedAt: now},
			},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "CI") {
		t.Errorf("active workflow missing from output: %q", out)
	}
	if strings.Contains(out, "Old") {
		t.Errorf("disabled workflow should be hidden by default: %q", out)
	}
}

func TestListAllIncludesDisabled(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.WorkflowsResponse{
			Workflows: []actions.Workflow{
				{ID: 1, Name: "CI", State: "active"},
				{ID: 2, Name: "Old", State: "disabled_manually"},
			},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		All:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "Old") {
		t.Errorf("--all should include disabled: %q", out)
	}
}

func TestListJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/workflows", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.WorkflowsResponse{
			Workflows: []actions.Workflow{{ID: 7, Name: "CI", State: "active"}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
	}
	opts.Exporter.JSONFields = "id,name"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), `"id":7`) {
		t.Errorf("json missing id: %s", tf.Out.String())
	}
}
