// SPDX-License-Identifier: AGPL-3.0-or-later

package delete

import (
	"context"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestDeleteWithYes(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r/issues/1")
}

func TestDeleteRequiresConfirmationWithoutYes(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: non-TTY delete needs --yes")
	}
}

func TestDeleteConfirmationMismatch(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.IOStreams.SetStdoutTTY(true)
	tf.Prompt.QueueInput("99")
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestDeleteConfirmationMatches(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.IOStreams.SetStdoutTTY(true)
	tf.Prompt.QueueInput("1")
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/o/r/issues/1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Arg:         "1",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
