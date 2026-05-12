// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUserUnmarshalAcceptsLogin covers the gh-canonical wire shape —
// what shithub S50 §1 is expected to emit.
func TestUserUnmarshalAcceptsLogin(t *testing.T) {
	var u User
	if err := json.Unmarshal([]byte(`{"id":7,"login":"octocat","name":"Octo Cat"}`), &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.Login != "octocat" || u.ID != 7 || u.Name != "Octo Cat" {
		t.Errorf("got %+v", u)
	}
}

// TestUserUnmarshalFallsBackToUsername covers the transitional shape the
// shithub server emits today (`username`). The fallback in UnmarshalJSON
// promotes it into Login so callers only ever read one field.
func TestUserUnmarshalFallsBackToUsername(t *testing.T) {
	var u User
	if err := json.Unmarshal([]byte(`{"id":7,"username":"octocat"}`), &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.Login != "octocat" {
		t.Errorf("Login should fall back from username; got %q", u.Login)
	}
}

// TestUserUnmarshalPrefersLogin: when both fields are present (during a
// migration window where server emits both), `login` wins.
func TestUserUnmarshalPrefersLogin(t *testing.T) {
	var u User
	if err := json.Unmarshal([]byte(`{"id":7,"login":"canonical","username":"legacy"}`), &u); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u.Login != "canonical" {
		t.Errorf("Login should prefer the canonical field; got %q", u.Login)
	}
}

// TestCurrentUserRejectsEmptyLogin covers audit #159: a server response
// missing both `login` and `username` must surface as an error rather
// than silently propagate an empty-Login struct that downstream
// ExpandMe / comment-owner callers would substitute into URLs.
func TestCurrentUserRejectsEmptyLogin(t *testing.T) {
	t.Setenv(EnvInsecureHTTP, "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":7,"name":"No Login"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(ClientOptions{
		BaseURL:   srv.URL,
		TokenFunc: func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	u, err := c.CurrentUser(context.Background())
	if err == nil {
		t.Fatalf("expected error for empty login; got %+v", u)
	}
	if !strings.Contains(err.Error(), "no login or username") {
		t.Errorf("error should mention missing login/username; got: %v", err)
	}
}
