// SPDX-License-Identifier: AGPL-3.0-or-later

package edit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

func TestEditTitle(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Title:       "new title",
		titleSet:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var p map[string]any
	_ = json.Unmarshal(body, &p)
	if p["title"] != "new title" {
		t.Errorf("title not patched: %v", p)
	}
}

func TestEditAddRemoveLabelsMutate(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues/1", 200, issues.Issue{
		Number: 1, Labels: []issues.Label{{Name: "bug"}, {Name: "old"}},
	})
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1})
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		Arg:          "1",
		Repo:         "o/r",
		AddLabels:    []string{"new"},
		RemoveLabels: []string{"old"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var p map[string]any
	_ = json.Unmarshal(body, &p)
	labels, _ := p["labels"].([]any)
	want := []string{"bug", "new"}
	if len(labels) != len(want) {
		t.Fatalf("labels: got %v want %v", labels, want)
	}
	for i, v := range labels {
		if v != want[i] {
			t.Errorf("labels[%d] got %v want %s", i, v, want[i])
		}
	}
}

func TestEditMutexFlags(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Labels:      []string{"a"},
		AddLabels:   []string{"b"},
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: --label + --add-label mutex")
	}
}

func TestEditNoFlagsErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when no flags set")
	}
}

func TestEditRemoveMilestone(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1})
	})

	opts := &options{
		IO:              tf.IOStreams,
		HTTPClient:      tf.Factory.HTTPClient,
		DefaultHost:     tf.Factory.DefaultHost,
		Arg:             "1",
		Repo:            "o/r",
		RemoveMilestone: true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, hasMilestone := func() (any, bool) {
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		v, ok := m["milestone"]
		return v, ok
	}(); !hasMilestone {
		t.Errorf("expected milestone=0 in PATCH body: %s", body)
	}
}
