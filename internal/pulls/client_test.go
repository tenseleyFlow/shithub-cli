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

func TestCreate(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PR{Number: 7, Title: "hi"})
	})

	c := NewClient(srv.NewClient())
	out, err := c.Create(context.Background(), "o", "r", CreateInput{
		Title: "hi", Body: "hello", Head: "feature", Base: "trunk", Draft: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.Number != 7 {
		t.Errorf("got %+v", out)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["draft"] != true {
		t.Errorf("draft not sent: %v", sent)
	}
	if sent["head"] != "feature" || sent["base"] != "trunk" {
		t.Errorf("head/base: %v", sent)
	}
}

func TestEditOnlySendsSetFields(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPatch, "/api/v1/repos/o/r/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PR{Number: 1})
	})

	c := NewClient(srv.NewClient())
	draft := false
	if _, err := c.Edit(context.Background(), "o", "r", 1, EditInput{Draft: &draft}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if _, ok := got["title"]; ok {
		t.Errorf("nil pointer field 'title' should be omitted: %v", got)
	}
	if got["draft"] != false {
		t.Errorf("draft not patched: %v", got)
	}
}

func TestUpdateBranchRebaseHeader(t *testing.T) {
	srv := fakeapi.New(t)
	var seenStrategy string
	srv.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/update-branch", func(w http.ResponseWriter, r *http.Request) {
		seenStrategy = r.Header.Get("X-Shithub-Strategy")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UpdateBranchResult{Message: "ok"})
	})

	c := NewClient(srv.NewClient())
	if _, err := c.UpdateBranch(context.Background(), "o", "r", 1, UpdateBranchInput{}, true); err != nil {
		t.Fatalf("UpdateBranch: %v", err)
	}
	if seenStrategy != "rebase" {
		t.Errorf("rebase header not set: %q", seenStrategy)
	}
}

func TestDiffEndpointReturnsRawBytes(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/o/r/pulls/1.diff", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("diff --git a/x b/x\n"))
	})

	c := NewClient(srv.NewClient())
	raw, err := c.Diff(context.Background(), "o", "r", 1)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.HasPrefix(string(raw), "diff --git") {
		t.Errorf("diff: %q", raw)
	}
}

func TestListMergedFilteredClientSide(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		// state=all because the server doesn't have a "merged" filter
		if r.URL.Query().Get("state") != "all" {
			t.Errorf("expected state=all, got %q", r.URL.Query().Get("state"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]PR{
			{Number: 1, Merged: true, State: "closed"},
			{Number: 2, Merged: false, State: "closed"},
			{Number: 3, Merged: true, State: "closed"},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.List(context.Background(), "o", "r", ListOptions{State: "merged"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("merged filter: got %v", out)
	}
}

func TestListDraftFilter(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]PR{
			{Number: 1, Draft: true},
			{Number: 2, Draft: false},
		})
	})

	c := NewClient(srv.NewClient())
	draftOnly := true
	out, err := c.List(context.Background(), "o", "r", ListOptions{Draft: &draftOnly})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 1 || out[0].Number != 1 {
		t.Errorf("draft filter: %v", out)
	}
}

func TestListEncodesBaseAndLabels(t *testing.T) {
	srv := fakeapi.New(t)
	var query string
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]PR{})
	})

	c := NewClient(srv.NewClient())
	_, err := c.List(context.Background(), "o", "r", ListOptions{
		State: "open", Base: "trunk", Labels: []string{"bug", "ux"},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, want := range []string{"state=open", "base=trunk", "labels=bug%2Cux"} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q: %s", want, query)
		}
	}
}

func TestListFiles(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/1/files", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]File{
			{Filename: "a.go", Status: "modified", Additions: 3, Deletions: 1, Changes: 4},
			{Filename: "b.md", Status: "added", Additions: 7, Deletions: 0, Changes: 7},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListFiles(context.Background(), "o", "r", 1)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(out) != 2 || out[0].Filename != "a.go" {
		t.Errorf("files: %v", out)
	}
}

func TestListCommits(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/1/commits", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"sha":"abc","commit":{"message":"first"}},{"sha":"def","commit":{"message":"second"}}]`))
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListCommits(context.Background(), "o", "r", 1)
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(out) != 2 || out[0].SHA != "abc" {
		t.Errorf("commits: %v", out)
	}
}
