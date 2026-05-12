// SPDX-License-Identifier: AGPL-3.0-or-later

package clone

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/labels"
)

func TestCloneFreshDest(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/src/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug", Color: "ff0000", Description: "broken"},
			{Name: "ux", Color: "00ff00"},
		})
	})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/dest/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{})
	})
	var created int32
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/dest/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&created, 1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Source:      "src/r",
		Repo:        "dest/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := atomic.LoadInt32(&created); got != 2 {
		t.Errorf("expected 2 created, got %d", got)
	}
}

func TestCloneSkipsCollisionsByDefault(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/src/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug", Color: "ff0000"},
		})
	})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/dest/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug", Color: "0000ff"},
		})
	})
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/dest/r/labels", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("POST should not fire when collision exists without --force")
	})
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/dest/r/labels/bug", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("PATCH should not fire without --force")
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Source:      "src/r",
		Repo:        "dest/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "skipped bug") {
		t.Errorf("expected skipped warning: %s", tf.ErrOut.String())
	}
}

func TestCloneForceOverwrites(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/src/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug", Color: "ff0000"},
		})
	})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/dest/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug", Color: "0000ff"},
		})
	})
	patched := false
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/dest/r/labels/bug", func(w http.ResponseWriter, _ *http.Request) {
		patched = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"bug","color":"ff0000"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Source:      "src/r",
		Repo:        "dest/r",
		Force:       true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !patched {
		t.Error("expected PATCH on collision with --force")
	}
}
