// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

func TestListRendersTable(t *testing.T) {
	tf := cmdutiltest.New(t)
	now := time.Now()
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Issue{
			{Number: 1, Title: "first", State: "open", UpdatedAt: now, Labels: []issues.Label{{Name: "bug"}}},
			{Number: 2, Title: "second", State: "open", UpdatedAt: now},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		State:       "open",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "#1") || !strings.Contains(out, "first") {
		t.Errorf("table missing issue: %q", out)
	}
}

func TestListAppliesFiltersOnQuery(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})

	var query string
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Issue{})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		State:       "open",
		Labels:      []string{"bug"},
		Assignee:    "@me",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"state=open", "labels=bug", "assignee=mf"} {
		if !strings.Contains(query, want) {
			t.Errorf("query missing %q: %s", want, query)
		}
	}
}

func TestListJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Issue{{Number: 1, Title: "t", State: "open"}})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		State:       "open",
		Limit:       DefaultLimit,
	}
	opts.Exporter.JSONFields = "number,title,state"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), `"number":1`) {
		t.Errorf("json missing: %s", tf.Out.String())
	}
}

// G3 (F1): `--json number` must actually project — the response body
// should contain ONLY the requested keys, not every field the
// exporter happens to build. Pre-fix the listing emitted the full
// gh-compat map and downstream `jq` pipelines broke on the unrequested
// keys. Pins the contract at the command layer where the audit caught
// it (output.Export's unit test pins the helper itself; this test
// pins that the helper gets called from the daily-driver list code).
func TestListJSONExportProjectsRequestedFieldsOnly(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]issues.Issue{
			{Number: 1, Title: "t", State: "open", Body: "long body that should not surface"},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		State:       "open",
		Limit:       DefaultLimit,
	}
	opts.Exporter.JSONFields = "number"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	// number requested → present.
	if !strings.Contains(out, `"number":1`) {
		t.Errorf("number missing from projection: %s", out)
	}
	// title NOT requested → absent.
	if strings.Contains(out, `"title"`) {
		t.Errorf("title leaked into projection (only number requested): %s", out)
	}
	// body NOT requested → absent.
	if strings.Contains(out, `"body"`) {
		t.Errorf("body leaked into projection: %s", out)
	}
}

// TestListRoundtrip_AuthorWireShape pins the wire-shape the CLI sends
// for `--author`. F-audit F11 / F2 status table tracks the divergence:
// the server expects `?author=` (and 422s on unknown user), the CLI
// today sends `?creator=` so the validation is bypassed.
//
// This test runs the cobra command end-to-end through cmdutil's flag
// parser (where wire-name bugs hide) using the new RunCobra +
// AssertQueryParam helpers from G16. When G1 lands its fix, flip the
// expected key from `creator` to `author` and the regression is
// locked in at the unit-test layer.
func TestListRoundtrip_AuthorWireShape(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues", 200, []issues.Issue{})

	err := tf.RunCobra(t, NewCmd(tf.Factory),
		"--author", "ghost", "-R", "o/r")
	if err != nil {
		t.Fatalf("RunCobra: %v", err)
	}

	// CURRENT behaviour (F11 unfixed): CLI sends `creator=ghost`.
	// G1 flips this to `author=ghost`; update the assertions then.
	tf.Server.AssertQueryParam(http.MethodGet, "/api/v1/repos/o/r/issues", "creator", "ghost")
	tf.Server.AssertQueryAbsent(http.MethodGet, "/api/v1/repos/o/r/issues", "author")
}

func TestListWebSkipsAPI(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Web:         true,
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/issues") {
		t.Errorf("URL wrong: %q", opened)
	}
}

// C9 regression: Run rejects nonsensical --limit values with the
// gh-compatible "invalid limit: N" message instead of silently
// coercing them away.
func TestListRejectsInvalidLimit(t *testing.T) {
	tf := cmdutiltest.New(t)
	cases := []int{0, -1, -100}
	for _, n := range cases {
		opts := &options{
			IO:          tf.IOStreams,
			HTTPClient:  tf.Factory.HTTPClient,
			DefaultHost: tf.Factory.DefaultHost,
			Opener:      func(string) error { return nil },
			Repo:        "o/r",
			Limit:       n,
		}
		err := Run(context.Background(), opts)
		if err == nil {
			t.Errorf("Limit=%d: want error, got nil", n)
			continue
		}
		if !strings.Contains(err.Error(), "invalid limit:") {
			t.Errorf("Limit=%d: got %q, want 'invalid limit:' prefix", n, err.Error())
		}
	}
}

func TestHumanAgeBranches(t *testing.T) {
	now := time.Now()
	cases := map[time.Duration]string{
		0:                  "just now",
		2 * time.Minute:    "2m ago",
		3 * time.Hour:      "3h ago",
		2 * 24 * time.Hour: "2d ago",
	}
	for d, want := range cases {
		got := humanAge(now.Add(-d))
		if got != want {
			t.Errorf("d=%v: got %q want %q", d, got, want)
		}
	}
}
