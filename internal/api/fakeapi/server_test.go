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
