// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/orgs"
)

func TestRunRendersTable(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/orgs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]orgs.Org{
			{Login: "tenseleyFlow", Role: "admin", PublicRepos: 4, MembersCount: 3, IsVerified: true},
			{Login: "other", Role: "member", Suspended: true},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"tenseleyFlow", "admin", "4 repos, 3 members", "verified", "other", "member", "suspended"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in table:\n%s", want, out)
		}
	}
}

func TestRunPublicUserOrgs(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/users/octocat/orgs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]orgs.Org{{Login: "public-org"}})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		User:        "octocat",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "public-org") {
		t.Errorf("table missing public-org: %s", tf.Out.String())
	}
}

func TestRunEmptyMessage(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/orgs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]orgs.Org{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "no organizations found") {
		t.Errorf("ErrOut: %q", tf.ErrOut.String())
	}
}

func TestRunJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/orgs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]orgs.Org{{Login: "tenseleyFlow", Role: "admin"}})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Limit:       DefaultLimit,
	}
	opts.Exporter.JSONSet = true
	opts.Exporter.JSONFields = "login,role"
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(tf.Out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\nraw: %s", err, tf.Out.String())
	}
	if got[0]["login"] != "tenseleyFlow" || got[0]["role"] != "admin" {
		t.Errorf("got %v", got)
	}
}
