// SPDX-License-Identifier: AGPL-3.0-or-later

package updatebranch

import (
	"context"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

func TestUpdateBranchDefaultMerge(t *testing.T) {
	tf := cmdutiltest.New(t)
	var seenStrategy string
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/update-branch", func(w http.ResponseWriter, r *http.Request) {
		seenStrategy = r.Header.Get("X-Shithub-Strategy")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})
	_ = pulls.PR{}

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
	if seenStrategy != "" {
		t.Errorf("merge strategy should not set rebase header: %q", seenStrategy)
	}
}

func TestUpdateBranchRebase(t *testing.T) {
	tf := cmdutiltest.New(t)
	var seenStrategy string
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/pulls/1/update-branch", func(w http.ResponseWriter, r *http.Request) {
		seenStrategy = r.Header.Get("X-Shithub-Strategy")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Rebase:      true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenStrategy != "rebase" {
		t.Errorf("rebase header: %q", seenStrategy)
	}
}
