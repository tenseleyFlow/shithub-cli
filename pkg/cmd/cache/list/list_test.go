// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestListRendersCacheEntries(t *testing.T) {
	tf := cmdutiltest.New(t)
	now := time.Now()
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/caches", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.CachesResponse{
			TotalCount: 1,
			ActionsCaches: []actions.Cache{
				{ID: 5, Key: "Linux-go-abc123", Ref: "refs/heads/trunk", SizeInBytes: 4096, LastAccessedAt: now},
			},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Sort:        "last_accessed_at",
		Order:       "desc",
		Limit:       30,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"5", "Linux-go-abc123", "refs/heads/trunk"} {
		if !strings.Contains(out, want) {
			t.Errorf("row missing %q: %s", want, out)
		}
	}
}

func TestListPassesQueryFilters(t *testing.T) {
	tf := cmdutiltest.New(t)
	var query string
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/caches", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(actions.CachesResponse{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Key:         "Linux-go-",
		Ref:         "refs/heads/trunk",
		Sort:        "size_in_bytes",
		Order:       "asc",
		Limit:       30,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"key=Linux-go-", "ref=refs%2Fheads%2Ftrunk", "sort=size_in_bytes", "direction=asc"} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q: %s", want, query)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		512:                    "512 B",
		2048:                   "2.0 KiB",
		3 * 1024 * 1024:        "3.0 MiB",
		2 * 1024 * 1024 * 1024: "2.0 GiB",
	}
	for n, want := range cases {
		got := humanSize(n)
		if got != want {
			t.Errorf("humanSize(%d): got %q want %q", n, got, want)
		}
	}
}
