// SPDX-License-Identifier: AGPL-3.0-or-later

package sync

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestSyncHappyPath(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/me/hello", 200, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"}, DefaultBranch: "trunk", Fork: true,
	})
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos/me/hello/merge-upstream", 200, repos.MergeUpstreamResult{
		Message: "Successfully fetched and fast-forwarded from upstream", MergeType: "fast-forward", BaseBranch: "trunk",
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "me/hello",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "fast-forward") && !strings.Contains(tf.ErrOut.String(), "Successfully") {
		t.Errorf("expected success message; got %q", tf.ErrOut.String())
	}
}

func TestSyncBranchOverride(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos/me/hello/merge-upstream", 200, repos.MergeUpstreamResult{
		Message: "ok", MergeType: "fast-forward",
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "me/hello",
		Branch:      "feature",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// We sent the explicit branch, so no metadata fetch should be needed.
	for _, c := range tf.Server.Calls() {
		if c.Method == "GET" && strings.HasSuffix(c.Path, "/me/hello") {
			t.Errorf("unexpected metadata fetch when --branch passed")
		}
	}
}

func TestSyncServerErrorPropagates(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/me/hello", 200, repos.Repo{
		Name: "hello", FullName: "me/hello", Owner: repos.Owner{Login: "me"}, DefaultBranch: "trunk", Fork: true,
	})
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/me/hello/merge-upstream", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"non-fast-forward"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "me/hello",
		// No GitRunner -> local fallback is skipped, error is surfaced.
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when server rejects merge-upstream and no local fallback available")
	}
}
