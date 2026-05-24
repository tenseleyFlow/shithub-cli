// SPDX-License-Identifier: AGPL-3.0-or-later

package crosskind_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/pkg/cmd/shared/crosskind"
)

func TestCheck_IssueExpectedFoundPR(t *testing.T) {
	tf := cmdutiltest.New(t)
	// Wrong-side: caller wanted issue#3 but #3 is a PR.
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls/3", 200, pulls.PR{Number: 3})

	c := tf.Factory.HTTPClient
	client, _ := c("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	err := crosskind.Check(context.Background(), ic, pc, "o", "r", 3, "issue", "issue close", "close")
	var w *crosskind.ErrWrongNamespace
	if !errors.As(err, &w) {
		t.Fatalf("want ErrWrongNamespace, got %v", err)
	}
	if w.Found != "pr" || w.Number != 3 {
		t.Errorf("Found=%q Number=%d", w.Found, w.Number)
	}
	if !strings.Contains(err.Error(), "shithub pr close 3") {
		t.Errorf("message should suggest `shithub pr close 3`; got %q", err.Error())
	}
}

func TestCheck_PRExpectedFoundIssue(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues/1", 200, issues.Issue{Number: 1})

	client, _ := tf.Factory.HTTPClient("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	err := crosskind.Check(context.Background(), ic, pc, "o", "r", 1, "pr", "pr ready", "ready")
	var w *crosskind.ErrWrongNamespace
	if !errors.As(err, &w) {
		t.Fatalf("want ErrWrongNamespace, got %v", err)
	}
	if w.Found != "issue" {
		t.Errorf("Found=%q want issue", w.Found)
	}
	if !strings.Contains(err.Error(), "shithub issue ready 1") {
		t.Errorf("message should suggest `shithub issue ready 1`; got %q", err.Error())
	}
}

// TestCheck_NotFoundReturnsNil pins the "let the downstream call
// surface the not-found" contract: when neither side has the number,
// Check returns nil (caller's mutation handles it).
func TestCheck_NotFoundReturnsNil(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	})
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls/99", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	})

	client, _ := tf.Factory.HTTPClient("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	if err := crosskind.Check(context.Background(), ic, pc, "o", "r", 99, "pr", "pr close", "close"); err != nil {
		t.Errorf("non-existent number: want nil, got %v", err)
	}
}

// TestCheck_ExpectedSideReturnsNil pins the happy path: when the number
// IS the expected kind, return nil so the caller proceeds.
func TestCheck_ExpectedSideReturnsNil(t *testing.T) {
	tf := cmdutiltest.New(t)
	// PR side has #5, issue side doesn't.
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/issues/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	})

	client, _ := tf.Factory.HTTPClient("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	if err := crosskind.Check(context.Background(), ic, pc, "o", "r", 5, "pr", "pr close", "close"); err != nil {
		t.Errorf("expected-side: want nil, got %v", err)
	}
}

func TestCheck_ZeroNumberReturnsNil(t *testing.T) {
	tf := cmdutiltest.New(t)
	client, _ := tf.Factory.HTTPClient("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)
	if err := crosskind.Check(context.Background(), ic, pc, "o", "r", 0, "issue", "issue close", "close"); err != nil {
		t.Errorf("zero number: want nil, got %v", err)
	}
}

func TestIsWrongNamespace(t *testing.T) {
	w := &crosskind.ErrWrongNamespace{Number: 3, CmdName: "x", Found: "pr"}
	if !crosskind.IsWrongNamespace(w) {
		t.Error("direct sentinel not detected")
	}
	wrapped := errors.New("outer: " + w.Error())
	if crosskind.IsWrongNamespace(wrapped) {
		t.Error("plain wrapped string should not be detected")
	}
	if crosskind.IsWrongNamespace(nil) {
		t.Error("nil should not be detected")
	}
}

// TestCheckAsymmetric_PRReadyOnIssue pins audit-I6: an asymmetric
// PR-only verb on an issue number must NOT suggest the matching
// `issue <verb>` command because it doesn't exist. The error
// explains the asymmetry and steers at `issue view N` instead.
func TestCheckAsymmetric_PRReadyOnIssue(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues/1", 200, issues.Issue{Number: 1})

	client, _ := tf.Factory.HTTPClient("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	err := crosskind.CheckAsymmetric(context.Background(), ic, pc, "o", "r", 1, "pr", "pr ready", "ready", true)
	var w *crosskind.ErrWrongNamespace
	if !errors.As(err, &w) {
		t.Fatalf("want ErrWrongNamespace, got %v", err)
	}
	if !w.Asymmetric {
		t.Error("Asymmetric flag should be set")
	}
	msg := err.Error()
	if strings.Contains(msg, "shithub issue ready 1") {
		t.Errorf("must NOT point at non-existent `issue ready`: %q", msg)
	}
	if !strings.Contains(msg, "only applies to pull requests") {
		t.Errorf("expected asymmetry explanation: %q", msg)
	}
	if !strings.Contains(msg, "shithub issue view 1") {
		t.Errorf("expected view-redirect fallback: %q", msg)
	}
}

// TestCheckAsymmetric_PRMergeOnIssue is the same shape for `pr merge`.
func TestCheckAsymmetric_PRMergeOnIssue(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues/1", 200, issues.Issue{Number: 1})

	client, _ := tf.Factory.HTTPClient("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	err := crosskind.CheckAsymmetric(context.Background(), ic, pc, "o", "r", 1, "pr", "pr merge", "merge", true)
	if err == nil {
		t.Fatal("expected ErrWrongNamespace")
	}
	if strings.Contains(err.Error(), "shithub issue merge") {
		t.Errorf("must NOT point at non-existent `issue merge`: %v", err)
	}
}

// TestCheck_StillSuggestsForSymmetricVerb confirms the symmetric path
// (`pr close`, `pr edit`) keeps the existing "try `shithub issue X N`"
// redirect — those verbs DO exist on the other side.
func TestCheck_StillSuggestsForSymmetricVerb(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/issues/1", 200, issues.Issue{Number: 1})

	client, _ := tf.Factory.HTTPClient("")
	ic := issues.NewClient(client)
	pc := pulls.NewClient(client)

	err := crosskind.Check(context.Background(), ic, pc, "o", "r", 1, "pr", "pr close", "close")
	if err == nil {
		t.Fatal("expected ErrWrongNamespace")
	}
	if !strings.Contains(err.Error(), "shithub issue close 1") {
		t.Errorf("symmetric verb should still redirect: %v", err)
	}
}
