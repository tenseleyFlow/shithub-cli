// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestViewRendersHeaderAndJobs(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/runs/42", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.WorkflowRun{
			ID: 42, RunNumber: 7, Name: "CI",
			Status: "completed", Conclusion: "success",
			HeadBranch: "trunk", Event: "push", HTMLURL: "http://srv/o/r/runs/42",
		})
	})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/runs/42/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.JobsResponse{
			Jobs: []actions.Job{
				{
					ID: 9, Name: "build", Status: "completed", Conclusion: "success",
					Steps: []actions.Step{{Name: "checkout", Status: "completed", Conclusion: "success"}},
				},
			},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		RunID:       "42",
		ShowJobs:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"run #7", "trunk", "build", "checkout"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
}

func TestViewExitStatusOnFailure(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/runs/42", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.WorkflowRun{
			ID: 42, RunNumber: 7, Name: "CI",
			Status: "completed", Conclusion: "failure",
		})
	})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/runs/42/jobs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.JobsResponse{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		RunID:       "42",
		ExitStatus:  true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("Run: expected error for failed run with --exit-status")
	}
}

func TestViewRejectsNonNumericID(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		RunID:       "abc",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("Run: expected parse error for non-numeric run id")
	}
}
