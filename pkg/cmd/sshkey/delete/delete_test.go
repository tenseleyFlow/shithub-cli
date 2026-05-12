// SPDX-License-Identifier: AGPL-3.0-or-later

package del

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

func TestDeleteByID(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodDelete, "/api/v1/user/keys/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Selector:    "42",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodDelete, "/api/v1/user/keys/42")
}

// TestDeleteByTitleResolvesID covers the audit-flagged convenience: a
// non-numeric selector hits ListSSH first, finds the matching title,
// then deletes by id.
func TestDeleteByTitleResolvesID(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]keys.SSHKey{
			{ID: 7, Title: "laptop"},
			{ID: 9, Title: "yubikey"},
		})
	})
	tf.Server.Handle(http.MethodDelete, "/api/v1/user/keys/9", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Selector:    "yubikey",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodDelete, "/api/v1/user/keys/9")
}

func TestDeleteRequiresYesInNonInteractive(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Selector:    "1",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: --yes required in non-interactive mode")
	}
}
