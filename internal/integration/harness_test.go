// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Env-var names that opt into integration runs. The host is the bare
// scheme://host[:port] (no /api/v1 suffix); the token is a PAT issued
// against that host.
const (
	envHost  = "SHITHUB_INTEGRATION_HOST"
	envToken = "SHITHUB_INTEGRATION_TOKEN" //nolint:gosec // env var name, not a credential
)

// requireIntegration skips the calling test when the substrate env vars
// aren't set. Returns the configured host + token so the test can build
// its own clients.
func requireIntegration(t *testing.T) (host, token string) {
	t.Helper()
	host = os.Getenv(envHost)
	token = os.Getenv(envToken)
	if host == "" || token == "" {
		t.Skipf("set %s and %s to run integration tests", envHost, envToken)
	}
	return host, token
}

// newClient builds an api.Client targeting the configured integration
// host. The candidate-token flow from internal/auth isn't used here —
// integration tests are read-only against the wire, not exercising the
// keyring layer.
func newClient(t *testing.T, host, token string) *api.Client {
	t.Helper()
	c, err := api.NewClient(api.ClientOptions{
		Host: host,
		TokenFunc: func(_ context.Context, _ string) (string, string, error) {
			return token, "integration", nil
		},
		Timeout:    10 * time.Second,
		MaxRetries: api.IntPtr(1),
	})
	if err != nil {
		t.Fatalf("api.NewClient: %v", err)
	}
	return c
}
