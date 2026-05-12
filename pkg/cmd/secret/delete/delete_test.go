// SPDX-License-Identifier: AGPL-3.0-or-later

package delete

import (
	"context"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestDeleteRepoSecretIssuesDELETE(t *testing.T) {
	tf := cmdutiltest.New(t)
	called := false
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/o/r/actions/secrets/FOO", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "FOO",
		Repo:        "o/r",
		App:         "actions",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Error("DELETE not issued")
	}
}

func TestDeleteOrgSecretIssuesDELETE(t *testing.T) {
	tf := cmdutiltest.New(t)
	called := false
	tf.Server.Handle(http.MethodDelete, "/api/v1/orgs/acme/actions/secrets/DEPLOY", func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "DEPLOY",
		Org:         "acme",
		App:         "actions",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Error("org DELETE not issued")
	}
}
