// SPDX-License-Identifier: AGPL-3.0-or-later

package browse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestBrowseNoBrowserPrintsURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "",
		Repo:        "o/r",
		NoBrowser:   true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "/o/r") {
		t.Errorf("stdout: %q", tf.Out.String())
	}
}

func TestBrowseNumberToPullURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "42",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/pulls/42") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseIssueHint(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "issue/9",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/issues/9") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseFileWithBranch(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "README.md",
		Repo:        "o/r",
		Branch:      "dev",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/blob/dev/README.md") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseCommitFlag(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Commit:      "abc1234",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/commit/abc1234") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseCommitMutexWithPositional(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "README.md",
		Repo:        "o/r",
		Commit:      "abc1234",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected --commit + positional mutex error")
	}
}

func TestBrowseTabsMutuallyExclusive(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Settings:    true,
		Wiki:        true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected tab mutex error")
	}
}

func TestBrowseSettingsTab(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Settings:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/settings") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseFallsBackOnOpenerFailure(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return errors.New("sandbox: no opener") },
		Arg:         "42",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "/o/r/pulls/42") {
		t.Errorf("fallback URL should be on stdout: %q", tf.Out.String())
	}
	if !strings.Contains(tf.ErrOut.String(), "opener failed") {
		t.Errorf("warning missing: %q", tf.ErrOut.String())
	}
}

// TestBrowseNumberAutoDetectsIssueWhenNotPR covers the audit #135 fix:
// `shithub browse 42` against an issue (not a PR) used to silently emit
// /pull/42 and 404. With the auto-detect probe, a 404 from the PR
// endpoint redirects the URL to /issues/N.
func TestBrowseNumberAutoDetectsIssueWhenNotPR(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/42", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "42",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/issues/42") {
		t.Errorf("expected issues URL after PR 404, got %q", opened)
	}
}

// TestBrowseNumberPreservesPRWhenExists confirms the happy path: the
// PR exists, the URL points at /pull/N.
func TestBrowseNumberPreservesPRWhenExists(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/42", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pulls.PR{Number: 42, State: "open"})
	})

	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "42",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/pulls/42") {
		t.Errorf("expected pull URL when PR exists, got %q", opened)
	}
}

// TestBrowseNumberFallsBackOnDetectFailure: a non-404 error from the
// PR view (auth, network) must not break browse — fall through to
// /pull/N so the user still gets a URL.
func TestBrowseNumberFallsBackOnDetectFailure(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	})

	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "42",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/pulls/42") {
		t.Errorf("expected pull fallback on detect failure, got %q", opened)
	}
}

func TestBrowseTreeWithBranchAlone(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Branch:      "feature/x",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/tree/feature/x") {
		t.Errorf("URL: %q", opened)
	}
}
