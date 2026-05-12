// SPDX-License-Identifier: AGPL-3.0-or-later

package create

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/labels"
)

func TestCreate(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(labels.Label{Name: "bug", Color: "ff0000"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
		Color:       "f00",
		Description: "broken stuff",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"color":"ff0000"`) {
		t.Errorf("color expansion: %s", body)
	}
	if !strings.Contains(string(body), `"description":"broken stuff"`) {
		t.Errorf("description: %s", body)
	}
}

func TestCreateRejectsBadColor(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
		Color:       "red",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected color validation error")
	}
}

func TestCreateForceFallsBackToEdit(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"validation failed"}`))
	})
	var patchBody json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/labels/bug", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		patchBody = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(labels.Label{Name: "bug", Color: "ff0000"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
		Color:       "ff0000",
		Description: "now better",
		Force:       true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(patchBody), `"color":"ff0000"`) {
		t.Errorf("PATCH body: %s", patchBody)
	}
}

func TestCreateNoForcePropagatesConflict(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"validation failed"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
		Color:       "ff0000",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: conflict without --force")
	}
}
