// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/labels"
)

func TestListRendersTable(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug", Color: "ff0000", Description: "broken stuff"},
			{Name: "enhancement", Color: "00ff00"},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Sort:        "name",
		Direction:   "asc",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"bug", "#ff0000", "enhancement", "broken stuff"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestListSearchFilter(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug"}, {Name: "enhancement"}, {Name: "buglet"},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Search:      "bug",
		Sort:        "name",
		Limit:       DefaultLimit,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "bug") || !strings.Contains(out, "buglet") {
		t.Errorf("missing match: %s", out)
	}
	if strings.Contains(out, "enhancement") {
		t.Errorf("non-match leaked: %s", out)
	}
}

func TestListJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]labels.Label{
			{Name: "bug", Color: "ff0000"},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Repo:        "o/r",
		Limit:       DefaultLimit,
	}
	opts.Exporter.JSONFields = "name,color"
	opts.Exporter.JSONSet = true
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), `"name":"bug"`) || !strings.Contains(tf.Out.String(), `"color":"ff0000"`) {
		t.Errorf("json: %s", tf.Out.String())
	}
}
