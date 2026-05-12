// SPDX-License-Identifier: AGPL-3.0-or-later

package commits

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestCommitsQualifiers(t *testing.T) {
	tf := cmdutiltest.New(t)
	var gotQ string
	tf.Server.Handle(http.MethodGet, "/api/v1/search/commits", func(w http.ResponseWriter, r *http.Request) {
		gotQ = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count":1,"items":[{"sha":"abc1234","commit":{"message":"fix bug"}}]}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "fix",
		Author:      "octocat",
		NoMerge:     true,
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"fix", "author:octocat", "repo:o/r", "merge:false"} {
		if !strings.Contains(gotQ, want) {
			t.Errorf("q = %q; missing %q", gotQ, want)
		}
	}
	if !strings.Contains(tf.Out.String(), "abc1234") {
		t.Errorf("missing short SHA: %s", tf.Out.String())
	}
}

func TestCommitsMutex(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Query:       "x",
		Merge:       true,
		NoMerge:     true,
	}
	if err := Run(nil, opts); err == nil { //nolint:staticcheck // mutex check fires before ctx use
		t.Fatal("want mutex error")
	}
}
