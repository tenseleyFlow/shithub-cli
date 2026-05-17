// SPDX-License-Identifier: AGPL-3.0-or-later

package pulls

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestMergeSquash(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MergeResult{Merged: true, SHA: "abc"})
	})

	c := NewClient(srv.NewClient())
	out, err := c.Merge(context.Background(), "o", "r", 1, MergeInput{
		MergeMethod: MergeSquash, CommitTitle: "Merge title",
	}, false)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !out.Merged {
		t.Errorf("merged: %+v", out)
	}
	if !strings.Contains(string(body), `"merge_method":"squash"`) {
		t.Errorf("strategy: %s", body)
	}
}

// TestMergeCommitSHA_GhCompat confirms the decoder accepts gh's
// `merge_commit_sha` field and exposes it through CommitSHA(). The
// CLI success line went blank on shithub server builds that emitted
// only this field (B-audit B1).
func TestMergeCommitSHA_GhCompat(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"merged":true,"merge_commit_sha":"feedface"}`))
	})

	c := NewClient(srv.NewClient())
	out, err := c.Merge(context.Background(), "o", "r", 1, MergeInput{MergeMethod: MergeMerge}, false)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got := out.CommitSHA(); got != "feedface" {
		t.Errorf("CommitSHA: got %q, want feedface (raw: %+v)", got, out)
	}
}

// TestMergeCommitSHA_PrefersGhCompat: when both fields are populated
// (legacy + gh-compat), the gh-compat name wins so the CLI doesn't
// display two different shas across server versions.
func TestMergeCommitSHA_PrefersGhCompat(t *testing.T) {
	r := &MergeResult{SHA: "legacy", MergeCommitSHA: "ghcompat"}
	if r.CommitSHA() != "ghcompat" {
		t.Errorf("CommitSHA: got %q, want ghcompat", r.CommitSHA())
	}
}

func TestMergeAdminHeader(t *testing.T) {
	srv := fakeapi.New(t)
	var seen string
	srv.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Shithub-Admin-Override")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MergeResult{Merged: true})
	})

	c := NewClient(srv.NewClient())
	if _, err := c.Merge(context.Background(), "o", "r", 1, MergeInput{MergeMethod: MergeMerge}, true); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if seen != "1" {
		t.Errorf("admin header: %q", seen)
	}
}

func TestEnableDisableAutoMerge(t *testing.T) {
	srv := fakeapi.New(t)
	var seenBody json.RawMessage
	srv.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/auto-merge", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenBody = b
		w.WriteHeader(http.StatusOK)
	})
	srv.Handle(http.MethodDelete, "/api/v1/repos/o/r/pulls/1/auto-merge", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	c := NewClient(srv.NewClient())
	if err := c.EnableAutoMerge(context.Background(), "o", "r", 1, AutoMergeInput{
		MergeMethod: MergeSquash, CommitTitle: "Auto",
	}); err != nil {
		t.Fatalf("EnableAutoMerge: %v", err)
	}
	if !strings.Contains(string(seenBody), `"merge_method":"squash"`) {
		t.Errorf("auto-merge body: %s", seenBody)
	}
	if err := c.DisableAutoMerge(context.Background(), "o", "r", 1); err != nil {
		t.Fatalf("DisableAutoMerge: %v", err)
	}
	srv.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r/pulls/1/auto-merge")
}

func TestDeleteBranch(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodDelete, "/api/v1/repos/o/r/git/refs/heads/feature", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	c := NewClient(srv.NewClient())
	if err := c.DeleteBranch(context.Background(), "o", "r", "feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	srv.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r/git/refs/heads/feature")
}
