// SPDX-License-Identifier: AGPL-3.0-or-later

package prs

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/search"
	searchshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/shared"
)

func TestPRsSendsTypePRAndExtras(t *testing.T) {
	tf := cmdutiltest.New(t)
	var gotQ, gotType string
	tf.Server.Handle(http.MethodGet, "/api/v1/search/issues", func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		gotType = r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(search.Response[pulls.PR]{
			TotalCount: 1,
			Items: []pulls.PR{{
				Number: 12, Title: "feat",
				Head: pulls.Ref{Ref: "feature/x"},
				Base: pulls.Ref{Ref: "trunk"},
			}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "feature",
		Issue:       searchshared.IssueFlags{State: "open"},
		NoDraft:     true,
		Merged:      true,
		Checks:      "passing",
		Base:        "trunk",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotType != "pr" {
		t.Errorf("type: %q", gotType)
	}
	for _, want := range []string{"feature", "state:open", "is:merged", "-is:draft", "status:passing", "base:trunk"} {
		if !strings.Contains(gotQ, want) {
			t.Errorf("q = %q; missing %q", gotQ, want)
		}
	}
}

func TestPRsMutexErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	cases := []struct {
		name string
		opts *options
	}{
		{"draft", &options{IO: tf.IOStreams, HTTPClient: tf.Factory.HTTPClient, DefaultHost: tf.Factory.DefaultHost, Opener: func(_ string) error { return nil }, Query: "x", Draft: true, NoDraft: true}},
		{"merged", &options{IO: tf.IOStreams, HTTPClient: tf.Factory.HTTPClient, DefaultHost: tf.Factory.DefaultHost, Opener: func(_ string) error { return nil }, Query: "x", Merged: true, NoMerged: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Run(context.Background(), tc.opts); err == nil {
				t.Fatalf("want mutex error for %s", tc.name)
			}
		})
	}
}
