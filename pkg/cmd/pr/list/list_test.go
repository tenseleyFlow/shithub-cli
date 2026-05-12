// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestListRendersTable(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]pulls.PR{
			{Number: 1, Title: "first", State: "open", Head: pulls.Ref{Ref: "feature"}},
			{Number: 2, Title: "second", State: "closed", Merged: true, Head: pulls.Ref{Ref: "fix"}},
			{Number: 3, Title: "third", State: "open", Draft: true, Head: pulls.Ref{Ref: "draft-branch"}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		State:       "open",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"#1", "first", "merged", "draft", "feature"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

func TestListSendsBaseAndLabels(t *testing.T) {
	tf := cmdutiltest.New(t)
	var query string
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]pulls.PR{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		State:       "merged",
		Base:        "trunk",
		Labels:      []string{"bug"},
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// state=merged maps to state=all on the wire (client-side filter).
	for _, want := range []string{"state=all", "base=trunk", "labels=bug"} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q: %s", want, query)
		}
	}
}

func TestListDraftFilter(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]pulls.PR{
			{Number: 1, Draft: true, Head: pulls.Ref{Ref: "a"}, Title: "draft-only"},
			{Number: 2, Draft: false, Head: pulls.Ref{Ref: "b"}, Title: "ready"},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Draft:       "true",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "draft-only") {
		t.Errorf("missing draft PR: %s", tf.Out.String())
	}
	if strings.Contains(tf.Out.String(), "ready") {
		t.Errorf("ready PR should be filtered out: %s", tf.Out.String())
	}
}

func TestListJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]pulls.PR{
			{Number: 1, Title: "first", State: "open", Head: pulls.Ref{Ref: "x"}, Base: pulls.Ref{Ref: "trunk"}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		State:       "open",
		Limit:       DefaultLimit,
	}
	opts.Exporter.JSONFields = "number,title,headRefName,baseRefName"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{`"number":1`, `"headRefName":"x"`, `"baseRefName":"trunk"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json missing %q: %s", want, out)
		}
	}
}

func TestListWebSkipsAPI(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Web:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/pulls") {
		t.Errorf("URL: %q", opened)
	}
}
