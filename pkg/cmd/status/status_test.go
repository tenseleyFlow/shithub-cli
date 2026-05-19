// SPDX-License-Identifier: AGPL-3.0-or-later

package status

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
)

// fixedTime returns a stable now() for the mentions window so the
// query string is deterministic across test runs.
func fixedTime() time.Time { return time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC) }

// stubWhoami patches the /user endpoint to return the given login.
func stubWhoami(t *testing.T, srv interface {
	Handle(method, path string, h http.HandlerFunc)
}, login string,
) {
	t.Helper()
	srv.Handle(http.MethodGet, "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(api.User{Login: login, ID: 1})
	})
}

func TestRunMergesFourSections(t *testing.T) {
	tf := cmdutiltest.New(t)
	stubWhoami(t, tf.Server, "octocat")

	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("q")
		typ := r.URL.Query().Get("type")
		switch {
		case typ == "issue" && strings.Contains(q, "assignee:octocat"):
			_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{Items: []issues.Issue{
				{Number: 1, Title: "fix lint", Repository: &issues.RepoRef{FullName: "o/r"}, UpdatedAt: fixedTime()},
			}})
		case typ == "pr" && strings.Contains(q, "assignee:octocat"):
			_ = json.NewEncoder(w).Encode(search.Response[pulls.PR]{Items: []pulls.PR{
				{Number: 2, Title: "feat", Repository: &issues.RepoRef{FullName: "o/r"}, UpdatedAt: fixedTime()},
			}})
		case typ == "pr" && strings.Contains(q, "review-requested:octocat"):
			_ = json.NewEncoder(w).Encode(search.Response[pulls.PR]{Items: []pulls.PR{
				{Number: 3, Title: "rev me", Repository: &issues.RepoRef{FullName: "o/r"}, UpdatedAt: fixedTime()},
			}})
		case typ == "issue" && strings.Contains(q, "mentions:octocat"):
			_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{Items: []issues.Issue{
				{Number: 4, Title: "ping", Repository: &issues.RepoRef{FullName: "o/r"}, UpdatedAt: fixedTime()},
			}})
		default:
			t.Errorf("unexpected query: type=%q q=%q", typ, q)
		}
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		now:         fixedTime,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"Hello @octocat", "Assigned Issues (1)", "Assigned Pull Requests (1)", "Review Requests (1)", "Mentions (1)", "o/r#1", "o/r#2", "o/r#3", "o/r#4"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRunHidesEmptySectionsByDefault(t *testing.T) {
	tf := cmdutiltest.New(t)
	stubWhoami(t, tf.Server, "octocat")
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		now:         fixedTime,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, hide := range []string{"Assigned Issues", "Assigned Pull Requests", "Review Requests", "Mentions"} {
		if strings.Contains(tf.Out.String(), hide) {
			t.Errorf("empty section %q should be hidden; got:\n%s", hide, tf.Out.String())
		}
	}
}

func TestRunShowEmptyShowsAllSections(t *testing.T) {
	tf := cmdutiltest.New(t)
	stubWhoami(t, tf.Server, "octocat")
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		ShowEmpty:   true,
		now:         fixedTime,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"Assigned Issues (0)", "Assigned Pull Requests (0)", "Review Requests (0)", "Mentions (0)", "(none)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q; got:\n%s", want, out)
		}
	}
}

func TestRunExcludeAndOrgFiltersInQuery(t *testing.T) {
	tf := cmdutiltest.New(t)
	stubWhoami(t, tf.Server, "octocat")
	var seen atomic.Pointer[string]
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		s := r.URL.Query().Get("q")
		seen.Store(&s)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Org:         "tenseleyFlow",
		Exclude:     []string{"spam"},
		now:         fixedTime,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	p := seen.Load()
	if p == nil {
		t.Fatal("server never received a query")
	}
	if !strings.Contains(*p, "org:tenseleyFlow") || !strings.Contains(*p, "-org:spam") {
		t.Errorf("query missing filters: %q", *p)
	}
}

func TestRunPartialFailureRendersWhatItHas(t *testing.T) {
	tf := cmdutiltest.New(t)
	stubWhoami(t, tf.Server, "octocat")
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("q")
		typ := r.URL.Query().Get("type")
		if typ == "issue" && strings.Contains(q, "mentions:octocat") {
			// Simulate server pain for the mentions section only.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{Items: []issues.Issue{
			{Number: 1, Title: "ok", Repository: &issues.RepoRef{FullName: "o/r"}, UpdatedAt: fixedTime()},
		}})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		now:         fixedTime,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "mentions") {
		t.Errorf("expected mentions warning; got %q", tf.ErrOut.String())
	}
	if !strings.Contains(tf.Out.String(), "Assigned Issues (1)") {
		t.Errorf("expected other sections to render; got:\n%s", tf.Out.String())
	}
}

func TestRunJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	stubWhoami(t, tf.Server, "octocat")
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{Items: []issues.Issue{
			{Number: 7, Title: "x", Repository: &issues.RepoRef{FullName: "o/r"}, UpdatedAt: fixedTime()},
		}})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		now:         fixedTime,
	}
	opts.Exporter.JSONSet = true
	// G9c (F20): canonical camelCase JSON field names.
	opts.Exporter.JSONFields = "user,assignedIssues"
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(tf.Out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\nraw: %s", err, tf.Out.String())
	}
	if got["user"] != "octocat" {
		t.Errorf("user: %v", got["user"])
	}
	items, _ := got["assignedIssues"].([]any)
	if len(items) != 1 {
		t.Errorf("assignedIssues: %v", got["assignedIssues"])
	}
}

// G9c (F20): pin the new field-name contract. Each section uses
// gh-canonical camelCase (`assignedIssues`, `assignedPRs`,
// `reviewRequests`), matching every other --json exporter. Pre-fix
// the snake_case names rejected gh-ported scripts at the validator.
func TestStatusJSONFieldsAreCamelCase(t *testing.T) {
	exp := exporter{}
	got := exp.Fields()
	want := map[string]bool{
		"user":           true,
		"assignedIssues": true,
		"assignedPRs":    true,
		"reviewRequests": true,
		"mentions":       true,
	}
	if len(got) != len(want) {
		t.Fatalf("Fields(): want %d, got %d: %v", len(want), len(got), got)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("Fields() unexpected entry %q (snake_case would be %q-style)", f, "assigned_issues")
		}
		// Reject any snake_case (underscore-containing) entry — the
		// gh-canonical surface has none.
		if strings.Contains(f, "_") {
			t.Errorf("Fields(): %q contains underscore; expected camelCase", f)
		}
	}
}

// TestCollapseWarnings covers the audit A2 rollup. Identical errors
// across multiple sections collapse to "every section: <error>";
// distinct errors pass through unchanged.
func TestCollapseWarnings(t *testing.T) {
	t.Run("all four identical", func(t *testing.T) {
		got := collapseWarnings([]string{
			`assigned issues: token lacks scope "repo:read"`,
			`assigned PRs: token lacks scope "repo:read"`,
			`review requests: token lacks scope "repo:read"`,
			`mentions: token lacks scope "repo:read"`,
		})
		if len(got) != 1 || got[0] != `every section: token lacks scope "repo:read"` {
			t.Errorf("got %v", got)
		}
	})
	t.Run("distinct stay distinct", func(t *testing.T) {
		got := collapseWarnings([]string{
			"assigned issues: timed out",
			`mentions: token lacks scope "repo:read"`,
		})
		if len(got) != 2 {
			t.Errorf("expected 2 warnings, got %v", got)
		}
	})
	t.Run("nothing to collapse", func(t *testing.T) {
		got := collapseWarnings(nil)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}
