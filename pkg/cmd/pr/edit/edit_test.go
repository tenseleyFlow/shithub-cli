// SPDX-License-Identifier: AGPL-3.0-or-later

package edit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestEditTitleAndBase(t *testing.T) {
	tf := cmdutiltest.New(t)
	var prPatchBody json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		prPatchBody = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.PR{Number: 1})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Title:       "new title",
		titleSet:    true,
		Base:        "release",
		baseSet:     true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var p map[string]any
	_ = json.Unmarshal(prPatchBody, &p)
	if p["title"] != "new title" || p["base"] != "release" {
		t.Errorf("PR patch: %v", p)
	}
}

func TestEditAddRemoveLabelsMutate(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues/1", 200, issues.Issue{
		Number: 1, Labels: []issues.Label{{Name: "bug"}, {Name: "old"}},
	})
	var issuePatchBody json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		issuePatchBody = b
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
	_ = json.Unmarshal(issuePatchBody, &p)
	labels, _ := p["labels"].([]any)
	want := []string{"bug", "new"}
	if len(labels) != len(want) {
		t.Fatalf("labels: %v want %v", labels, want)
	}
	for i, v := range labels {
		if v != want[i] {
			t.Errorf("labels[%d] %v want %s", i, v, want[i])
		}
	}
}

func TestEditAddReviewersHitsRequestedReviewers(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodPatch, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{Number: 1})

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls/1/requested_reviewers", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.WriteHeader(http.StatusCreated)
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		Arg:          "1",
		Repo:         "o/r",
		Title:        "x",
		titleSet:     true,
		AddReviewers: []string{"alice,bob"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var p map[string]any
	_ = json.Unmarshal(body, &p)
	revs, _ := p["reviewers"].([]any)
	if len(revs) != 2 || revs[0] != "alice" || revs[1] != "bob" {
		t.Errorf("reviewers: %v", revs)
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
		t.Fatal("expected error: nothing to do")
	}
}

func TestEditAssigneesAtMeExpansion(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})

	var issuePatchBody json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		issuePatchBody = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issues.Issue{Number: 1})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Assignees:   []string{"@me", "octo"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var p map[string]any
	_ = json.Unmarshal(issuePatchBody, &p)
	a, _ := p["assignees"].([]any)
	if len(a) != 2 || a[0] != "mf" || a[1] != "octo" {
		t.Errorf("assignees expansion: %v", a)
	}
}

// TestEditRejectsBodyPlusBodyFile pins H16: pre-fix passing both
// --body and --body-file silently used the file and discarded the
// inline value. cobra's MarkFlagsMutuallyExclusive now refuses the
// combination at parse.
func TestEditRejectsBodyPlusBodyFile(t *testing.T) {
	tf := cmdutiltest.New(t)
	cmd := NewCmd(tf.Factory)
	cmd.SetArgs([]string{"1", "--repo", "o/r", "--body", "inline", "--body-file", "/tmp/x"})
	cmd.SetOut(tf.Out)
	cmd.SetErr(tf.ErrOut)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from mutex, got nil")
	}
}
