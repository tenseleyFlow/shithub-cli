// SPDX-License-Identifier: AGPL-3.0-or-later

package issues

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestCreateIssue(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Issue{Number: 1, Title: "hi"})
	})

	c := NewClient(srv.NewClient())
	out, err := c.Create(context.Background(), "o", "r", CreateInput{
		Title: "hi", Body: "hello", Labels: []string{"bug"}, Assignees: []string{"me"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if out.Number != 1 {
		t.Errorf("unexpected: %+v", out)
	}
	var sent map[string]any
	_ = json.Unmarshal(body, &sent)
	if sent["title"] != "hi" {
		t.Errorf("title not sent: %v", sent)
	}
	if labels, _ := sent["labels"].([]any); len(labels) != 1 || labels[0] != "bug" {
		t.Errorf("labels not sent: %v", sent)
	}
}

func TestEditOnlySendsSetFields(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPatch, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Issue{Number: 1, State: "closed"})
	})

	c := NewClient(srv.NewClient())
	state := "closed"
	if _, err := c.Edit(context.Background(), "o", "r", 1, EditInput{State: &state}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if _, ok := got["title"]; ok {
		t.Errorf("nil title should be omitted: %v", got)
	}
	if got["state"] != "closed" {
		t.Errorf("state not patched: %v", got)
	}
}

func TestLockSendsReason(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPut, "/api/v1/repos/o/r/issues/1/lock", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.WriteHeader(http.StatusNoContent)
	})

	c := NewClient(srv.NewClient())
	if err := c.Lock(context.Background(), "o", "r", 1, "spam"); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if !strings.Contains(string(body), `"lock_reason":"spam"`) {
		t.Errorf("reason not sent: %s", body)
	}
}

func TestUnlock(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodDelete, "/api/v1/repos/o/r/issues/1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c := NewClient(srv.NewClient())
	if err := c.Unlock(context.Background(), "o", "r", 1); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	srv.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r/issues/1/lock")
}

func TestAddListEditDeleteComment(t *testing.T) {
	srv := fakeapi.New(t)
	srv.RegisterJSON(http.MethodPost, "/api/v1/repos/o/r/issues/1/comments", 201, Comment{ID: 7, Body: "hi"})
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Comment{{ID: 7, Body: "hi"}})
	})
	srv.RegisterJSON(http.MethodPatch, "/api/v1/repos/o/r/issues/comments/7", 200, Comment{ID: 7, Body: "edited"})
	srv.Handle(http.MethodDelete, "/api/v1/repos/o/r/issues/comments/7", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	c := NewClient(srv.NewClient())
	out, err := c.AddComment(context.Background(), "o", "r", 1, "hi")
	if err != nil || out.ID != 7 {
		t.Fatalf("AddComment: out=%+v err=%v", out, err)
	}
	list, err := c.ListComments(context.Background(), "o", "r", 1)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListComments: %v %v", list, err)
	}
	edited, err := c.EditComment(context.Background(), "o", "r", 7, "edited")
	if err != nil || edited.Body != "edited" {
		t.Fatalf("EditComment: %v %v", edited, err)
	}
	if err := c.DeleteComment(context.Background(), "o", "r", 7); err != nil {
		t.Fatalf("DeleteComment: %v", err)
	}
}

func TestListAppliesFilters(t *testing.T) {
	srv := fakeapi.New(t)
	var seenQuery string
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]Issue{{Number: 1}})
	})

	c := NewClient(srv.NewClient())
	_, err := c.List(context.Background(), "o", "r", ListOptions{
		State: "open", Labels: []string{"bug", "ux"}, Assignee: "mf",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, want := range []string{"state=open", "labels=bug%2Cux", "assignee=mf"} {
		if !strings.Contains(seenQuery, want) {
			t.Errorf("query missing %q: %s", want, seenQuery)
		}
	}
}

// TestListAcrossReposScopes pins F29: ListAcrossRepos now translates
// the gh-style scope set onto /search/issues qualifiers. The legacy
// `/issues?filter=...` endpoint doesn't exist on shithub yet, and the
// audit endorsed the search-based path until it ships.
func TestListAcrossReposScopes(t *testing.T) {
	srv := fakeapi.New(t)
	var seenQuery string
	srv.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []Issue{{Number: 1}}})
	})
	c := NewClient(srv.NewClient())

	// `created` → author:@me qualifier
	if _, err := c.ListAcrossRepos(context.Background(), "created", ListOptions{State: "open"}); err != nil {
		t.Fatalf("ListAcrossRepos created: %v", err)
	}
	if !strings.Contains(seenQuery, "author%3A%40me") || !strings.Contains(seenQuery, "state%3Aopen") {
		t.Errorf("created query wrong: %s", seenQuery)
	}

	// `assigned` → assignee:@me qualifier
	if _, err := c.ListAcrossRepos(context.Background(), "assigned", ListOptions{State: "open"}); err != nil {
		t.Fatalf("ListAcrossRepos assigned: %v", err)
	}
	if !strings.Contains(seenQuery, "assignee%3A%40me") {
		t.Errorf("assigned query wrong: %s", seenQuery)
	}

	// `mentioned` is deferred — should not hit the server at all.
	srv.Handle(http.MethodGet, "/api/v1/search/issues", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("mentioned scope must not round-trip; it's deferred")
	})
	items, err := c.ListAcrossRepos(context.Background(), "mentioned", ListOptions{State: "open"})
	if err != nil {
		t.Fatalf("ListAcrossRepos mentioned: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("mentioned should return nil/empty; got %v", items)
	}
}

func TestIsPullRequestFlag(t *testing.T) {
	i := Issue{PullRequest: &struct{}{}}
	if !i.IsPullRequest() {
		t.Error("expected PR")
	}
	plain := Issue{}
	if plain.IsPullRequest() {
		t.Error("plain issue should not be PR")
	}
}

func TestWhoamiCachesResult(t *testing.T) {
	srv := fakeapi.New(t)
	calls := 0
	srv.Handle(http.MethodGet, "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.User{Login: "mf"})
	})

	c := srv.NewClient()
	for i := 0; i < 3; i++ {
		u, err := c.CurrentUser(context.Background())
		if err != nil {
			t.Fatalf("CurrentUser: %v", err)
		}
		if u.Login != "mf" {
			t.Errorf("login: %q", u.Login)
		}
	}
	if calls != 1 {
		t.Errorf("expected single wire call from cache; got %d", calls)
	}
}
