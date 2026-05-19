// SPDX-License-Identifier: AGPL-3.0-or-later

package delete

import (
	"context"
	"net/http"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestDeleteWithYesFlag(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/o/r", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r")
}

func TestDeleteRequiresConfirmationWithoutYes(t *testing.T) {
	tf := cmdutiltest.New(t)
	// Non-TTY by default; no --yes -> hard error.
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: non-TTY delete needs --yes")
	}
}

func TestDeleteConfirmationMustMatch(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.IOStreams.SetStdoutTTY(true)
	tf.Prompt.QueueInput("wrong/name")

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestDeleteConfirmationMatches(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.IOStreams.SetStdoutTTY(true)
	tf.Prompt.QueueInput("o/r")
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/o/r", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodDelete, "/api/v1/repos/o/r")
}

// TestDeleteInfersCurrentUserOwner pins F38: an unqualified repo name
// resolves to the authenticated user's namespace, matching the rule
// `repo create <name>` already follows. Pre-fix this errored with
// "expected owner/name" while `create` worked.
func TestDeleteInfersCurrentUserOwner(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/user", 200, map[string]any{"login": "alice"})
	tf.Server.Handle(http.MethodDelete, "/api/v1/repos/alice/cli-audit-rt", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:          tf.IOStreams,
		Prompter:    tf.Prompt,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "cli-audit-rt",
		Yes:         true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodDelete, "/api/v1/repos/alice/cli-audit-rt")
}
