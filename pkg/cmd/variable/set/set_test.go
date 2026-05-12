// SPDX-License-Identifier: AGPL-3.0-or-later

package set

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
)

// TestSetRepoVariableCreatesWhenMissing — 404 on GET probe triggers POST.
func TestSetRepoVariableCreatesWhenMissing(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/variables/FOO", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	var posted secrets.SetVariableInput
	tf.Server.Handle(http.MethodPost, "/api/v1/repos/o/r/actions/variables", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &posted)
		w.WriteHeader(http.StatusCreated)
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "FOO",
		Repo:        "o/r",
		Body:        "bar",
		BodySet:     true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if posted.Name != "FOO" || posted.Value != "bar" {
		t.Errorf("POST body wrong: %+v", posted)
	}
}

// TestSetRepoVariableUpdatesWhenPresent — GET returns 200 → PATCH.
func TestSetRepoVariableUpdatesWhenPresent(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/actions/variables/FOO", 200,
		secrets.Variable{Name: "FOO", Value: "old"})
	var patched secrets.SetVariableInput
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r/actions/variables/FOO", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &patched)
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "FOO",
		Repo:        "o/r",
		Body:        "new",
		BodySet:     true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if patched.Value != "new" {
		t.Errorf("PATCH body wrong: %+v", patched)
	}
}
