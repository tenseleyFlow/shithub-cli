// SPDX-License-Identifier: AGPL-3.0-or-later

package merge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestMergeSquashDeletesBranch(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open",
		Head: pulls.Ref{Ref: "feature", SHA: "abc", Repo: &pulls.RepoLite{FullName: "o/r"}},
		Base: pulls.Ref{Ref: "trunk", Repo: &pulls.RepoLite{FullName: "o/r"}},
	})
	var body json.RawMessage
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.MergeResult{Merged: true, SHA: "deadbeefcafef00d"})
	})
	deleted := false
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/o/r/git/refs/heads/feature", func(w http.ResponseWriter, _ *http.Request) {
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		Arg:          "1",
		Repo:         "o/r",
		Squash:       true,
		DeleteBranch: true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"merge_method":"squash"`) {
		t.Errorf("strategy: %s", body)
	}
	if !deleted {
		t.Error("expected DELETE on /git/refs/heads/feature")
	}
}

// TestMergeAutoEnable + TestMergeDisableAuto were removed by H4: the
// server-side auto-merge endpoint isn't shipped (vapor flag, H-audit
// finding H6). The tests registered fake endpoints that masked the
// "not found" the user actually sees. Replaced by
// TestMergeAutoRejectedClientSide / TestMergeDisableAutoRejectedClientSide
// at the bottom of this file which pin the client-side reject.

func TestMergeMatchHeadMismatchErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "actualhead"},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Merge:       true,
		MatchHead:   "wronghead",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected head-mismatch error")
	}
}

func TestMergeStrategyMutex(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Merge:       true,
		Squash:      true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected mutex error")
	}
}

func TestMergeRefusesClosedOrMerged(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "closed", Head: pulls.Ref{SHA: "abc"},
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Merge:       true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: closed PR")
	}
}

func TestMergeAdminHeader(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})
	var seen string
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("X-Shithub-Admin-Override")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.MergeResult{Merged: true, SHA: "deadbeef"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Merge:       true,
		Admin:       true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seen != "1" {
		t.Errorf("admin header: %q", seen)
	}
}

func TestMergeDefaultStrategyFallsBackToMerge(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
	})
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{Name: "r", FullName: "o/r"})

	var body json.RawMessage
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.MergeResult{Merged: true, SHA: "abc"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"merge_method":"merge"`) {
		t.Errorf("default strategy: %s", body)
	}
}

// TestMergeDefaultStrategyHonorsRepoFlags walks through the gh
// preference order (merge → squash → rebase): for each repo
// configuration we omit any explicit --merge/--squash/--rebase flag and
// confirm the chooser picks the highest-precedence allowed method.
func TestMergeDefaultStrategyHonorsRepoFlags(t *testing.T) {
	cases := []struct {
		name string
		repo repos.Repo
		want string
	}{
		{"squash only", repos.Repo{AllowSquashMerge: true}, "squash"},
		{"rebase only", repos.Repo{AllowRebaseMerge: true}, "rebase"},
		{"merge allowed too", repos.Repo{AllowMergeCommit: true, AllowSquashMerge: true, AllowRebaseMerge: true}, "merge"},
		{"squash beats rebase", repos.Repo{AllowSquashMerge: true, AllowRebaseMerge: true}, "squash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tf := cmdutiltest.New(t)
			tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
				Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"},
			})
			tc.repo.Name, tc.repo.FullName = "r", "o/r"
			tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, tc.repo)

			var body json.RawMessage
			tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				body = b
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(pulls.MergeResult{Merged: true, SHA: "abc"})
			})

			opts := &options{
				IO:          tf.IOStreams,
				HTTPClient:  tf.Factory.HTTPClient,
				DefaultHost: tf.Factory.DefaultHost,
				Arg:         "1",
				Repo:        "o/r",
			}
			if err := Run(context.Background(), opts); err != nil {
				t.Fatalf("Run: %v", err)
			}
			want := `"merge_method":"` + tc.want + `"`
			if !strings.Contains(string(body), want) {
				t.Errorf("want %s in body, got %s", want, body)
			}
		})
	}
}

func TestMergeInsertPRBody(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/1", 200, pulls.PR{
		Number: 1, State: "open", Head: pulls.Ref{SHA: "abc"}, Body: "PR body text",
	})
	var body json.RawMessage
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/merge", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.MergeResult{Merged: true, SHA: "abc"})
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		Arg:          "1",
		Repo:         "o/r",
		Merge:        true,
		InsertPRBody: true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"commit_message":"PR body text"`) {
		t.Errorf("PR body not inserted: %s", body)
	}
}

// TestMergeAutoRejectedClientSide pins H6: `pr merge --auto` is a
// vapor flag — the server doesn't implement auto-merge. Pre-fix the
// CLI sent the request and the user got "shithub: not found" exit 1.
// Now the guard returns NotYetSupportedError, which the root maps to
// exit 2 so scripts can distinguish "feature pending" from a real
// failure.
func TestMergeAutoRejectedClientSide(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Auto:        true,
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected NotYetSupportedError")
	}
	if !cmdutil.IsNotYetSupported(err) {
		t.Errorf("not a NotYetSupportedError: %v", err)
	}
	// No request should have hit the server — the guard runs first.
	if len(tf.Server.Calls()) > 0 {
		t.Errorf("unexpected API call: %+v", tf.Server.Calls())
	}
}

func TestMergeDisableAutoRejectedClientSide(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		DisableAuto: true,
	}
	err := Run(context.Background(), opts)
	if !cmdutil.IsNotYetSupported(err) {
		t.Errorf("want NotYetSupportedError, got %v", err)
	}
}
