// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
)

func TestListRepoSecrets(t *testing.T) {
	tf := cmdutiltest.New(t)
	now := time.Now()
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/secrets", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(secrets.SecretsResponse{
			Secrets: []secrets.Secret{{Name: "FOO", UpdatedAt: now}, {Name: "BAR", UpdatedAt: now}},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		App:         "actions",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "FOO") || !strings.Contains(out, "BAR") {
		t.Errorf("table missing entries: %q", out)
	}
}

func TestListOrgSecrets(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/orgs/acme/actions/secrets", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(secrets.SecretsResponse{
			Secrets: []secrets.Secret{{Name: "DEPLOY", Visibility: "all"}},
		})
	})
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Org:         "acme",
		App:         "actions",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "DEPLOY") || !strings.Contains(out, "all") {
		t.Errorf("org-scope table missing visibility column: %q", out)
	}
}

func TestListRejectsUnsupportedApp(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		App:         "codespaces",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error for unsupported app")
	}
}
