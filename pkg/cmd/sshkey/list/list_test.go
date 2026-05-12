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
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

func TestListRenders(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]keys.SSHKey{
			{ID: 1, Title: "laptop", Kind: keys.SSHKindAuthentication, Fingerprint: "SHA256:abc", CreatedAt: time.Now()},
			{ID: 2, Title: "yubikey", Kind: keys.SSHKindSigning, Fingerprint: "SHA256:def", CreatedAt: time.Now()},
		})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, want := range []string{"laptop", "yubikey", "SHA256:abc", "authentication", "signing"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q: %s", want, out)
		}
	}
}

func TestListJSONExport(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]keys.SSHKey{{ID: 1, Title: "laptop", Kind: keys.SSHKindAuthentication}})
	})
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	opts.Exporter.JSONSet = true
	opts.Exporter.JSONFields = "id,title,kind"
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(tf.Out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got[0]["title"] != "laptop" || got[0]["kind"] != "authentication" {
		t.Errorf("export: %v", got)
	}
}
