// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestViewByNumber(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, Title: "hi", State: "open",
		User: &api.User{Login: "octo"},
		Head: pulls.Ref{Ref: "feature"}, Base: pulls.Ref{Ref: "trunk"},
		Labels: []issues.Label{{Name: "bug"}},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"#1", "hi", "octo", "base:trunk", "head:feature", "bug"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

func TestViewByBranchLookup(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]pulls.PR{
			{Number: 42, Head: pulls.Ref{Ref: "feature"}},
		})
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/42", 200, pulls.PR{
		Number: 42, Title: "from-branch", State: "open", Head: pulls.Ref{Ref: "feature"}, Base: pulls.Ref{Ref: "trunk"},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "feature",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "from-branch") {
		t.Errorf("branch lookup missed: %s", tf.Out.String())
	}
}

func TestViewIncludesComments(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, Title: "hi", State: "open", Head: pulls.Ref{Ref: "x"}, Base: pulls.Ref{Ref: "trunk"},
	})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/1/comments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Comment{
			{ID: 11, Body: "first thread comment", User: &api.User{Login: "a"}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "1",
		Repo:        "o/r",
		Comments:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "first thread comment") {
		t.Errorf("comment missing: %s", tf.Out.String())
	}
}

func TestViewWebSkipsAPI(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "1",
		Repo:        "o/r",
		Web:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// G4 (F32): pr view --web must open /pulls/{N} (plural). Pre-fix
	// it opened /pull/{N} (singular) and landed users on a 404.
	if !strings.HasSuffix(opened, "/o/r/pulls/1") {
		t.Errorf("URL: %q want suffix /o/r/pulls/1", opened)
	}
}
