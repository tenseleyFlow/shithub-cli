// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func sampleRepos() []repos.Repo {
	t := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	return []repos.Repo{
		{
			Name: "alpha", FullName: "u/alpha", Owner: repos.Owner{Login: "u"},
			DefaultBranch: "trunk", Description: "first", Stargazers: 5,
			UpdatedAt: t, Language: "Go",
		},
		{
			Name: "beta", FullName: "u/beta", Owner: repos.Owner{Login: "u"},
			DefaultBranch: "trunk", Description: "second", Stargazers: 1,
			UpdatedAt: t, Language: "Python", Archived: true,
		},
		{
			Name: "gamma", FullName: "u/gamma", Owner: repos.Owner{Login: "u"},
			DefaultBranch: "trunk", Stargazers: 0,
			UpdatedAt: t, Fork: true, Topics: []string{"cli"},
		},
	}
}

func TestListRendersTable(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleRepos())
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"u/alpha", "u/beta", "u/gamma"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q; got: %s", want, out)
		}
	}
}

func TestListNoArchivedExcludesArchived(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleRepos())
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		NoArchived:  true,
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if strings.Contains(out, "u/beta") {
		t.Errorf("--no-archived should omit beta; got %s", out)
	}
}

func TestListLanguageFilter(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleRepos())
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Language:    "go",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "u/alpha") {
		t.Errorf("expected alpha, got %s", out)
	}
	if strings.Contains(out, "u/beta") || strings.Contains(out, "u/gamma") {
		t.Errorf("language filter leak: %s", out)
	}
}

func TestListTopicFilter(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleRepos())
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Topic:       "cli",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "u/gamma") {
		t.Errorf("topic filter missed gamma: %s", out)
	}
	if strings.Contains(out, "u/alpha") {
		t.Errorf("topic filter leaked alpha: %s", out)
	}
}

func TestListRejectsConflictingFlags(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Fork:        true,
		SourceOnly:  true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: --fork and --source mutually exclusive")
	}
	opts = &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Archived:    true,
		NoArchived:  true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: --archived and --no-archived mutually exclusive")
	}
}

func TestListJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleRepos())
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Limit:       DefaultLimit,
	}
	opts.Exporter.JSONFields = "fullName,stargazers"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, `"fullName":"u/alpha"`) {
		t.Errorf("json projection missing alpha: %s", out)
	}
}

func TestListUserPositional(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/users/octo/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sampleRepos()[:1])
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Owner:       "octo",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodGet, "/api/v1/users/octo/repos")
}

func TestApplyLimit(t *testing.T) {
	tf := cmdutiltest.New(t)
	rs := sampleRepos()
	tf.Server.Handle(http.MethodGet, "/api/v1/user/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rs)
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Limit:       1,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "u/alpha") || strings.Contains(out, "u/beta") {
		t.Errorf("limit=1 not enforced: %s", out)
	}
}
