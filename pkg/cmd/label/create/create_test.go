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
		_, _ = w.Write([]byte(`{"error":"label name already taken on this repo"}`))
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

// TestCreateUniquenessConflictNoForce: 422 whose message indicates a
// uniqueness conflict ("already taken") should surface the friendly
// "already exists; pass --force" error.
func TestCreateUniquenessConflictNoForce(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"label name already taken on this repo"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
		Color:       "ff0000",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on uniqueness conflict")
	}
	if !strings.Contains(err.Error(), "already exists") || !strings.Contains(err.Error(), "--force") {
		t.Errorf("expected friendly 'already exists; --force' error, got: %v", err)
	}
}

// TestCreate409TreatedAsConflict: the new server contract uses 409 for
// uniqueness. Confirm we still map it to the friendly error.
func TestCreate409TreatedAsConflict(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"label name already taken"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "bug",
		Repo:        "o/r",
		Color:       "ff0000",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on 409")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected friendly 'already exists' error, got: %v", err)
	}
}

// TestCreate422LengthErrorNotMisclassified is the C6 regression test:
// a 422 about length / charset / color shape MUST NOT be rendered as
// "already exists". Audit C6 caught this happening for "name length
// 1-50", "name too short", "color must be 3 or 6 hex digits", etc.
// The raw server message should reach the user.
func TestCreate422LengthErrorNotMisclassified(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"issues: label name length 1-50"}`))
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "x", // single char triggers the server's length check in the fixture
		Repo:        "o/r",
		Color:       "ff0000",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on length-422")
	}
	if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "--force") {
		t.Errorf("C6 regression: length-422 misclassified as already-exists; got: %v", err)
	}
	if !strings.Contains(err.Error(), "length") {
		t.Errorf("expected raw server message containing 'length'; got: %v", err)
	}
}
