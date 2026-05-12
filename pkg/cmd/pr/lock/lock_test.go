// SPDX-License-Identifier: AGPL-3.0-or-later

package lock

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestLockSendsReason(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body []byte
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/issues/1/lock", func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Reason:      "resolved",
		lock:        true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"lock_reason":"resolved"`) {
		t.Errorf("reason not sent: %s", body)
	}
}

func TestLockRejectsInvalidReason(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Reason:      "bogus",
		lock:        true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
}

func TestUnlockHitsDelete(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/o/r/issues/1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		lock:        false,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r/issues/1/lock")
}
