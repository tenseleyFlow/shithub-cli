// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
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
