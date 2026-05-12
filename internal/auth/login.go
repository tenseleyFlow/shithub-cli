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

// User is the trimmed shape we need from GET /api/v1/user. Fields we
// don't consume are deliberately omitted so tests don't fixate on
// unrelated server quirks.
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
}

// ValidationResult is what Validate returns on success: the username the
// server agrees the token belongs to, plus the OAuth scopes the server
// reports via the X-OAuth-Scopes header (empty when the server doesn't
// advertise — pre S50 §1 servers, and as graceful degradation).
type ValidationResult struct {
	User   User
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

// decodeUser parses the validated /api/v1/user response.
func decodeUser(resp *http.Response) (User, error) {
	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return User{}, fmt.Errorf("auth: decode /user: %w", err)
	}
	if u.Username == "" {
		return User{}, fmt.Errorf("auth: /user returned empty username")
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
