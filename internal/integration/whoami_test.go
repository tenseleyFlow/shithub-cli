// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package integration

import (
	"context"
	"testing"
)

// TestWhoamiSmoke is the canonical integration probe: hit /api/v1/user
// against a real shithub server and assert the response decodes into
// the canonical api.User shape. If this fails, every other typed
// command is broken too — wire-shape mismatch usually surfaces here
// first. Audit #129's User.Login vs Username schism is exactly what
// this test would have caught before it reached `auth login`.
func TestWhoamiSmoke(t *testing.T) {
	host, token := requireIntegration(t)
	c := newClient(t, host, token)

	user, err := c.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser: %v", err)
	}
	if user == nil || user.Login == "" {
		t.Fatalf("/api/v1/user returned empty Login: %+v", user)
	}
	t.Logf("authenticated as %s (id=%d) against %s", user.Login, user.ID, host)
}
