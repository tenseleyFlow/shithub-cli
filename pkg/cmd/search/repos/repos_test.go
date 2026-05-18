// SPDX-License-Identifier: AGPL-3.0-or-later

package repos

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
	searchshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/shared"
)

func TestRunComposesQualifiersAndQuery(t *testing.T) {
	tf := cmdutiltest.New(t)
	var gotQ string
	tf.Server.Handle(http.MethodGet, "/api/v1/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[repos.Repo]{
			TotalCount: 1,
			Items:      []repos.Repo{{FullName: "octo/x"}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "octocat",
		Owner:       "tenseleyFlow",
		Language:    "go",
		Stars:       ">10",
		Topic:       []string{"cli", "search"},
		Common:      searchshared.CommonFlags{Limit: 30},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// All qualifiers must appear in the q string verbatim alongside the
	// user's free-text portion.
	for _, want := range []string{"octocat", "user:tenseleyFlow", "language:go", "stars:>=11", "topic:cli", "topic:search"} {
		if !strings.Contains(gotQ, want) {
			t.Errorf("q = %q; missing %q", gotQ, want)
		}
	}
}

func TestRunRequiresQueryOrFilter(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("want error for empty query+filters")
	}
}

func TestRunWebOpensSearchURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(u string) error { opened = u; return nil },
		Query:       "octocat",
		Common:      searchshared.CommonFlags{Limit: 30, Web: true},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(opened, "/search?") || !strings.Contains(opened, "type=repositories") {
		t.Errorf("opened %q", opened)
	}
}

func TestRunRendersEmptyResultsMessage(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/search/repositories", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[repos.Repo]{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "zzz",
		Common:      searchshared.CommonFlags{Limit: 30},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "no results found") {
		t.Errorf("err out: %q", tf.ErrOut.String())
	}
}

func TestRunBadRangeBubblesUp(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "x",
		Stars:       "not-a-range",
		Common:      searchshared.CommonFlags{Limit: 30},
	}
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "--stars") {
		t.Fatalf("want --stars error; got %v", err)
	}
}
