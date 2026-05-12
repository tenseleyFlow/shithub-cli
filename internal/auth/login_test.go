// SPDX-License-Identifier: AGPL-3.0-or-later

package auth_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
	"github.com/tenseleyFlow/shithub-cli/internal/auth"
)

func TestValidateHappyPath(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token shithub_pat_abc" {
			t.Errorf("Authorization header: got %q", got)
		}
		w.Header().Set("X-OAuth-Scopes", "repo:read, user:read")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1,"username":"mf","email":"m@example"}`))
	})

	client := srv.NewClientWithToken("shithub_pat_abc")
	got, err := auth.Validate(context.Background(), client)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Server emits `username` today (pre-S50-§1); api.User.UnmarshalJSON
	// promotes it into Login so the canonical handle field is populated.
	if got.User.Login != "mf" {
		t.Errorf("Login: got %q", got.User.Login)
	}
	if len(got.Scopes) != 2 {
		t.Fatalf("Scopes: want 2 got %d (%v)", len(got.Scopes), got.Scopes)
	}
	if got.Scopes[0] != "repo:read" || got.Scopes[1] != "user:read" {
		t.Errorf("Scopes order/values: %v", got.Scopes)
	}
}

func TestValidateRejects401(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	})

	client := srv.NewClientWithToken("shithub_pat_bogus")
	_, err := auth.Validate(context.Background(), client)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !api.IsAuthError(err) {
		t.Errorf("expected AuthError, got %T: %v", err, err)
	}
}

func TestValidateScopesMissingHeader(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1,"username":"mf"}`))
	})

	client := srv.NewClientWithToken("shithub_pat_abc")
	got, err := auth.Validate(context.Background(), client)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.Scopes != nil {
		t.Errorf("missing scope header should yield nil, got %v", got.Scopes)
	}
}

func TestValidateRejectsEmptyLogin(t *testing.T) {
	srv := fakeapi.New(t)
	srv.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1})

	client := srv.NewClientWithToken("shithub_pat_abc")
	_, err := auth.Validate(context.Background(), client)
	if err == nil {
		t.Fatal("expected error on empty login")
	}
	if !strings.Contains(err.Error(), "empty login") {
		t.Errorf("error should mention empty login, got: %v", err)
	}
}

// TestValidateAcceptsCanonicalLogin verifies the gh-canonical `login`
// field is decoded directly. Once shithub S50 §1 lands, the server emits
// this shape and the `username` fallback in api.User.UnmarshalJSON
// becomes dead-but-harmless.
func TestValidateAcceptsCanonicalLogin(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1,"login":"mf","name":"M F"}`))
	})

	client := srv.NewClientWithToken("shithub_pat_abc")
	got, err := auth.Validate(context.Background(), client)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.User.Login != "mf" {
		t.Errorf("Login: got %q", got.User.Login)
	}
}

func TestValidateRejectsNilClient(t *testing.T) {
	t.Parallel()
	_, err := auth.Validate(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error on nil client")
	}
}

func TestNewCandidateClient(t *testing.T) {
	// Smoke test that the helper builds a valid client; we don't make a
	// real network call here.
	c, err := auth.NewCandidateClient("shithub.sh", "shithub_pat_x")
	if err != nil {
		t.Fatalf("NewCandidateClient: %v", err)
	}
	if c == nil {
		t.Fatal("nil client returned")
	}
}
