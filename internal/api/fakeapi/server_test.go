// SPDX-License-Identifier: AGPL-3.0-or-later

package fakeapi_test

import (
	"context"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

// TestSmokeRegisterAndCall demonstrates the minimal usage every later
// sprint's tests will repeat: register a JSON response, build a client,
// issue a REST call, assert recorded interaction.
func TestSmokeRegisterAndCall(t *testing.T) {
	fake := fakeapi.New(t)
	fake.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{
		"id":       1,
		"username": "tester",
	})

	c := fake.NewClient()

	var got struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	}
	if err := c.REST(context.Background(), "GET", "/api/v1/user", nil, &got); err != nil {
		t.Fatalf("REST: %v", err)
	}
	if got.Username != "tester" {
		t.Errorf("decoded username: got %q", got.Username)
	}

	fake.AssertCalled("GET", "/api/v1/user")
	fake.AssertCallCount(1)
}

// TestQueryParamAssertionsCatchWireShape covers the new roundtrip-test
// helpers that G16 added so wire-name bugs (F-audit F11/F12) can be
// caught at the CLI test layer. The repro shape: a request to
// `/issues` carrying `author=ghost` — if a future refactor switches
// the CLI to send `creator=ghost`, AssertQueryParam("...", "author",
// "ghost") fails and the test catches the wire-name flip before it
// merges.
func TestQueryParamAssertionsCatchWireShape(t *testing.T) {
	fake := fakeapi.New(t)
	fake.RegisterJSON("GET", "/api/v1/repos/o/r/issues", 422, map[string]any{"error": "author: user not found"})

	c := fake.NewClient()
	// Simulate the CLI: REST helper with explicit query string. The
	// real `issues.Client.List` does this internally; this test pins
	// the shape directly so the helper itself is exercised.
	var ignored any
	_ = c.REST(context.Background(), "GET", "/api/v1/repos/o/r/issues?author=ghost", nil, &ignored)

	fake.AssertCalled("GET", "/api/v1/repos/o/r/issues")
	fake.AssertQueryParam("GET", "/api/v1/repos/o/r/issues", "author", "ghost")
	fake.AssertQueryAbsent("GET", "/api/v1/repos/o/r/issues", "creator")

	if got := fake.LastCall("GET", "/api/v1/repos/o/r/issues"); got == nil {
		t.Fatal("LastCall returned nil for known call")
	} else if got.Query != "author=ghost" {
		t.Errorf("LastCall.Query = %q, want author=ghost", got.Query)
	}
}
