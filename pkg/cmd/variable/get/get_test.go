// SPDX-License-Identifier: AGPL-3.0-or-later

package get

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
)

func TestGetRepoVariablePrintsValue(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/variables/FOO", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(secrets.Variable{Name: "FOO", Value: "bar"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "FOO",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "bar") {
		t.Errorf("stdout missing value: %q", tf.Out.String())
	}
}

func TestGetOrgVariablePrintsValue(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/orgs/acme/actions/variables/REGION", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(secrets.Variable{Name: "REGION", Value: "us-east", Visibility: "all"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "REGION",
		Org:         "acme",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "us-east") {
		t.Errorf("stdout missing value: %q", tf.Out.String())
	}
}
