// SPDX-License-Identifier: AGPL-3.0-or-later

// Package auth holds login-flow primitives shared across the auth
// subcommands. Validate runs a candidate token through GET /api/v1/user,
// returns the resolved username + scopes, and leaves persistence to the
// calling subcommand.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// ValidationResult is what Validate returns on success: the user envelope
// the server agrees the token belongs to, plus the OAuth scopes the
// server reports via the X-OAuth-Scopes header (empty when the server
// doesn't advertise — pre S50 §1 servers, and as graceful degradation).
//
// User reuses api.User so the CLI has a single decoder for /api/v1/user
// — earlier sprints had a parallel auth.User type that read a different
// JSON field name; collapsing them prevents silent drift.
type ValidationResult struct {
	User   api.User
	Scopes []string
}

// NewCandidateClient builds an api.Client that uses the supplied bearer
// token verbatim, regardless of what hosts.yml or the keyring contain.
// Use this from `auth login` to validate a token before persisting it;
// every other code path should go through cmdutil.Factory.HTTPClient.
func NewCandidateClient(host, token string) (*api.Client, error) {
	return api.NewClient(api.ClientOptions{
		Host: host,
		TokenFunc: func(_ context.Context, _ string) (string, string, error) {
			return token, "candidate", nil
		},
		MaxRetries: 1,
	})
}

// Validate hits GET /api/v1/user via client. Returns a typed error from
// the api package when the token is rejected so the calling command can
// surface a tailored hint (api.IsAuthError → "token rejected"; network
// error → "connection failed").
//
// The client argument decouples Validate from token storage and from
// the fakeapi test harness: production callers pass NewCandidateClient's
// output; tests pass a fakeapi-backed client.
func Validate(ctx context.Context, client *api.Client) (*ValidationResult, error) {
	if client == nil {
		return nil, fmt.Errorf("auth: nil client")
	}

	resp, err := client.RESTRaw(ctx, http.MethodGet, "user", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	user, err := decodeUser(resp)
	if err != nil {
		return nil, err
	}

	scopes := parseScopesHeader(resp.Header.Get("X-OAuth-Scopes"))
	return &ValidationResult{User: user, Scopes: scopes}, nil
}

// decodeUser parses the validated /api/v1/user response. api.User's
// UnmarshalJSON accepts both `login` (gh-canonical) and `username`
// (shithub's current wire shape) so this works across the S50 transition.
func decodeUser(resp *http.Response) (api.User, error) {
	var u api.User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return api.User{}, fmt.Errorf("auth: decode /user: %w", err)
	}
	if u.Login == "" {
		return api.User{}, fmt.Errorf("auth: /user returned empty login")
	}
	return u, nil
}

// parseScopesHeader splits the X-OAuth-Scopes header into a slice. Empty
// header returns nil so callers can range freely; whitespace tolerated.
func parseScopesHeader(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}
