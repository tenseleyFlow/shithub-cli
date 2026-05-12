// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/orgs"
)

func TestRunRendersProfile(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/orgs/tenseleyFlow", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orgs.Org{
			Login:        "tenseleyFlow",
			Name:         "Tenseley Flow",
			Description:  "shithub org",
			Location:     "the cloud",
			Email:        "ops@example.test",
			PublicRepos:  4,
			MembersCount: 7,
			CreatedAt:    time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Org:         "tenseleyFlow",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"Tenseley Flow", "@tenseleyFlow", "shithub org", "Location:", "the cloud", "Public repos: 4", "Members:", "7", "Created:", "2026-01-02"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRunWebOpensURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(u string) error { opened = u; return nil },
		Org:         "tenseleyFlow",
		Web:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/tenseleyFlow") {
		t.Errorf("opened: %q", opened)
	}
}

func TestRunSuspendedWarn(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/orgs/zombie", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orgs.Org{Login: "zombie", Suspended: true})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(_ string) error { return nil },
		Org:         "zombie",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "suspended") {
		t.Errorf("ErrOut: %q", tf.ErrOut.String())
	}
}
