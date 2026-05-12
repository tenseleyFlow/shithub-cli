// SPDX-License-Identifier: AGPL-3.0-or-later

package repos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestViewDecodesRepoEnvelope(t *testing.T) {
	srv := fakeapi.New(t)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	srv.RegisterJSON(http.MethodGet, "/api/v1/repos/octo/hello", 200, Repo{
		ID:            42,
		Name:          "hello",
		FullName:      "octo/hello",
		Owner:         Owner{Login: "octo", Type: "User"},
		Private:       false,
		Visibility:    "public",
		Description:   "test repo",
		DefaultBranch: "trunk",
		Stargazers:    7,
		HTMLURL:       "https://shithub.sh/octo/hello",
		CloneURL:      "https://shithub.sh/octo/hello.git",
		SSHURL:        "git@shithub.sh:octo/hello.git",
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	c := NewClient(srv.NewClient())
	got, err := c.View(context.Background(), "octo", "hello")
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got.FullName != "octo/hello" || got.DefaultBranch != "trunk" {
		t.Errorf("decoded shape wrong: %+v", got)
	}
	srv.AssertCalled(http.MethodGet, "/api/v1/repos/octo/hello")
}

func TestCreateUserPostsBody(t *testing.T) {
	srv := fakeapi.New(t)
	var seenBody json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/user/repos", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenBody = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"name":"r","full_name":"u/r","default_branch":"trunk","owner":{"login":"u"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
	})

	c := NewClient(srv.NewClient())
	out, err := c.CreateUser(context.Background(), CreateInput{
		Name:        "r",
		Description: "hello",
		Private:     true,
		AutoInit:    true,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if out.Name != "r" {
		t.Errorf("unexpected return: %+v", out)
	}
	var sent map[string]any
	if err := json.Unmarshal(seenBody, &sent); err != nil {
		t.Fatalf("body json: %v", err)
	}
	if sent["name"] != "r" || sent["private"] != true || sent["auto_init"] != true {
		t.Errorf("unexpected POST body: %v", sent)
	}
}

func TestEditOnlySendsSetFields(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPatch, "/api/v1/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"r","full_name":"o/r","default_branch":"trunk","owner":{"login":"o"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
	})

	c := NewClient(srv.NewClient())
	desc := "new"
	if _, err := c.Edit(context.Background(), "o", "r", EditInput{Description: &desc}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := got["name"]; ok {
		t.Errorf("nil pointer field 'name' should be omitted; body=%s", body)
	}
	if got["description"] != "new" {
		t.Errorf("description not patched: %v", got)
	}
}

func TestArchiveSendsArchivedTrue(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPatch, "/api/v1/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"r","full_name":"o/r","default_branch":"trunk","owner":{"login":"o"},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
	})

	c := NewClient(srv.NewClient())
	if err := c.Archive(context.Background(), "o", "r"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["archived"] != true {
		t.Errorf("archived flag not set: %v", got)
	}
}

func TestDeleteCalls(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodDelete, "/api/v1/repos/o/r", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c := NewClient(srv.NewClient())
	if err := c.Delete(context.Background(), "o", "r"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	srv.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r")
}

func TestForkPostsBody(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/repos/u/r/forks", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":2,"name":"r","full_name":"me/r","default_branch":"trunk","owner":{"login":"me"},"fork":true,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
	})

	c := NewClient(srv.NewClient())
	out, err := c.Fork(context.Background(), "u", "r", ForkInput{DefaultBranchOnly: true})
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	if out.FullName != "me/r" {
		t.Errorf("unexpected fork: %+v", out)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got["default_branch_only"] != true {
		t.Errorf("default_branch_only not sent: %v", got)
	}
}

func TestMergeUpstreamSendsBranch(t *testing.T) {
	srv := fakeapi.New(t)
	srv.RegisterJSON(http.MethodPost, "/api/v1/repos/me/r/merge-upstream", 200, MergeUpstreamResult{
		Message:    "Successfully fetched and fast-forwarded from upstream",
		MergeType:  "fast-forward",
		BaseBranch: "trunk",
	})

	c := NewClient(srv.NewClient())
	out, err := c.MergeUpstream(context.Background(), "me", "r", MergeUpstreamInput{Branch: "trunk"})
	if err != nil {
		t.Fatalf("MergeUpstream: %v", err)
	}
	if out.MergeType != "fast-forward" {
		t.Errorf("unexpected result: %+v", out)
	}
}

func TestReplaceTopicsRoundTrip(t *testing.T) {
	srv := fakeapi.New(t)
	srv.RegisterJSON(http.MethodPut, "/api/v1/repos/o/r/topics", 200, TopicsPayload{Names: []string{"go", "cli"}})

	c := NewClient(srv.NewClient())
	out, err := c.ReplaceTopics(context.Background(), "o", "r", []string{"go", "cli"})
	if err != nil {
		t.Fatalf("ReplaceTopics: %v", err)
	}
	if len(out) != 2 || out[0] != "go" || out[1] != "cli" {
		t.Errorf("topics: %v", out)
	}
}

func TestReadREADMEDecodesBase64(t *testing.T) {
	srv := fakeapi.New(t)
	raw := "# Hello\n"
	srv.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/readme", 200, README{
		Name:     "README.md",
		Path:     "README.md",
		Encoding: "base64",
		Content:  base64.StdEncoding.EncodeToString([]byte(raw)),
	})

	c := NewClient(srv.NewClient())
	meta, content, err := c.ReadREADME(context.Background(), "o", "r", "")
	if err != nil {
		t.Fatalf("ReadREADME: %v", err)
	}
	if meta.Name != "README.md" {
		t.Errorf("meta: %+v", meta)
	}
	if string(content) != raw {
		t.Errorf("content decoded wrong: %q", content)
	}
}

func TestListAuthenticatedPaginates(t *testing.T) {
	srv := fakeapi.New(t)
	page1 := []Repo{{ID: 1, Name: "a", FullName: "u/a", Owner: Owner{Login: "u"}, DefaultBranch: "trunk"}}
	page2 := []Repo{{ID: 2, Name: "b", FullName: "u/b", Owner: Owner{Login: "u"}, DefaultBranch: "trunk"}}

	// Single handler keyed by ?page= so the fakeapi router (path-only)
	// hands both pages back to us. First page advertises ?page=2 via Link.
	srv.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode(page2)
			return
		}
		w.Header().Set("Link", `<`+srv.URL()+`/api/v1/user/repos?page=2>; rel="next"`)
		_ = json.NewEncoder(w).Encode(page1)
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListAuthenticated(context.Background(), ListOptions{PerPage: 1})
	if err != nil {
		t.Fatalf("ListAuthenticated: %v", err)
	}
	if len(out) != 2 || out[0].Name != "a" || out[1].Name != "b" {
		t.Errorf("paginated list: %+v", out)
	}
}

func TestListAuthenticatedHonorsLimit(t *testing.T) {
	srv := fakeapi.New(t)
	page1 := []Repo{
		{ID: 1, Name: "a", FullName: "u/a", Owner: Owner{Login: "u"}, DefaultBranch: "trunk"},
		{ID: 2, Name: "b", FullName: "u/b", Owner: Owner{Login: "u"}, DefaultBranch: "trunk"},
		{ID: 3, Name: "c", FullName: "u/c", Owner: Owner{Login: "u"}, DefaultBranch: "trunk"},
	}
	srv.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page1)
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListAuthenticated(context.Background(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("ListAuthenticated: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("limit not enforced: %+v", out)
	}
}

func TestEncodeListQueryDropsEmpty(t *testing.T) {
	got := encodeListQuery(ListOptions{Visibility: "public", Sort: "updated"})
	if !strings.Contains(got, "visibility=public") || !strings.Contains(got, "sort=updated") {
		t.Errorf("expected v+sort in query, got %q", got)
	}
	if strings.Contains(got, "type=") {
		t.Errorf("empty type should not be in query: %q", got)
	}
}
