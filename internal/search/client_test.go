// SPDX-License-Identifier: AGPL-3.0-or-later

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestRepositoriesPassesQueryVerbatim(t *testing.T) {
	srv := fakeapi.New(t)
	var gotQ, gotSort, gotOrder string
	srv.Handle(http.MethodGet, "/api/v1/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		gotSort = r.URL.Query().Get("sort")
		gotOrder = r.URL.Query().Get("order")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response[RepoItem]{
			TotalCount: 1,
			Items:      []RepoItem{{Name: "go", FullName: "octocat/go"}},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.Repositories(context.Background(), "octocat language:go stars:>10", Options{Sort: "stars", Order: "desc"})
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	if gotQ != "octocat language:go stars:>10" {
		t.Errorf("q: %q", gotQ)
	}
	if gotSort != "stars" || gotOrder != "desc" {
		t.Errorf("sort/order: %q/%q", gotSort, gotOrder)
	}
	if out.TotalCount != 1 || len(out.Items) != 1 || out.Items[0].FullName != "octocat/go" {
		t.Errorf("decoded: %+v", out)
	}
}

func TestIssuesAndPRsForkType(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response[map[string]any]{
			TotalCount: 1,
			Items:      []map[string]any{{"type": r.URL.Query().Get("type")}},
		})
	})

	c := NewClient(srv.NewClient())
	if _, err := c.Issues(context.Background(), "bug", Options{}); err != nil {
		t.Fatalf("Issues: %v", err)
	}
	if _, err := c.PullRequests(context.Background(), "bug", Options{}); err != nil {
		t.Fatalf("PullRequests: %v", err)
	}

	srv.AssertCallCount(2)
	srv.AssertCalled("GET", "/api/v1/search/issues")
}

func TestPaginationFollowsLinkHeaderAndAppliesLimit(t *testing.T) {
	srv := fakeapi.New(t)
	page := 0
	srv.Handle(http.MethodGet, "/api/v1/search/code", func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		if page < 3 {
			// Build an absolute next-page URL pointing at ourselves.
			next := fmt.Sprintf(`<%s://%s%s?p=%d>; rel="next"`, "http", r.Host, r.URL.Path, page+1)
			w.Header().Set("Link", next)
		}
		_ = json.NewEncoder(w).Encode(Response[CodeItem]{
			TotalCount: 9,
			Items: []CodeItem{
				{Name: fmt.Sprintf("p%d-a.go", page), Path: "a"},
				{Name: fmt.Sprintf("p%d-b.go", page), Path: "b"},
				{Name: fmt.Sprintf("p%d-c.go", page), Path: "c"},
			},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.Code(context.Background(), "TODO", Options{Limit: 5})
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if len(out.Items) != 5 {
		t.Errorf("limit=5: got %d items", len(out.Items))
	}
	if out.TotalCount != 9 {
		t.Errorf("preserves first-page total_count: %d", out.TotalCount)
	}
}

func TestPerPageClamp(t *testing.T) {
	srv := fakeapi.New(t)
	var got string
	srv.Handle(http.MethodGet, "/api/v1/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("per_page")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response[RepoItem]{})
	})

	c := NewClient(srv.NewClient())
	if _, err := c.Repositories(context.Background(), "q", Options{PerPage: 500}); err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	if got != "100" {
		t.Errorf("per_page clamp: %q", got)
	}
}

func TestCommitsDecodes(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/search/commits", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"total_count": 1,
			"items": [{
				"sha": "abc1234",
				"commit": {"message": "fix bug"},
				"score": 0.7
			}]
		}`))
	})

	c := NewClient(srv.NewClient())
	out, err := c.Commits(context.Background(), "fix", Options{})
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].SHA != "abc1234" || !strings.Contains(out.Items[0].Commit.Message, "fix") {
		t.Errorf("decoded: %+v", out.Items)
	}
}

// Sanity check: the type aliases match concrete types so callers can
// assert without going through Response[T] indirection.
func TestRepoItemAlias(t *testing.T) {
	_ = RepoItem(repos.Repo{})
}
