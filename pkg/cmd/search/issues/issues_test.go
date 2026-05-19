// SPDX-License-Identifier: AGPL-3.0-or-later

package issues

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
	searchshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/shared"
)

func TestIssuesSendsTypeIssueAndQualifiers(t *testing.T) {
	tf := cmdutiltest.New(t)
	var gotQ, gotType string
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		gotType = r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{
			TotalCount: 1,
			Items:      []issues.Issue{{Number: 7, Title: "bug", Repository: &issues.RepoRef{FullName: "o/r"}}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Issue: searchshared.IssueFlags{
			State:  "open",
			Label:  []string{"bug"},
			Locked: false, NoLocked: true,
		},
		Common: searchshared.CommonFlags{Limit: 30},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotType != "issue" {
		t.Errorf("type: %q", gotType)
	}
	for _, want := range []string{"state:open", "label:bug", "is:unlocked"} {
		if !strings.Contains(gotQ, want) {
			t.Errorf("q = %q; missing %q", gotQ, want)
		}
	}
}

// TestIssuesRendersRepoColumn pins F50: each row's repo column must be
// populated from the `repository.full_name` envelope. Pre-G9a the
// server returned a flat `repo` field only and the row layout had an
// empty leading column.
func TestIssuesRendersRepoColumn(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[issues.Issue]{
			TotalCount: 1,
			Items: []issues.Issue{{
				Number: 1, State: "open", Title: "renamed issue",
				Repository: &issues.RepoRef{FullName: "mfwolffe/demo"},
			}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "renamed",
		Common:      searchshared.CommonFlags{Limit: 30},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"mfwolffe/demo", "#1", "open", "renamed issue"} {
		if !strings.Contains(tf.Out.String(), want) {
			t.Errorf("row missing %q; got %q", want, tf.Out.String())
		}
	}
}
