// SPDX-License-Identifier: AGPL-3.0-or-later

// Package device implements the CLI side of the RFC 8628 OAuth 2.0
// Device Authorization Grant against a shithub host. RequestCode hits
// POST /login/device/code; Exchange hits POST /login/oauth/access_token
// once; Poll wraps Exchange in a back-off loop honoring the protocol's
// `authorization_pending` / `slow_down` / `expired_token` responses.
//
// The endpoints are unauthenticated and form-encoded so this package
// does not reuse internal/api (which assumes a bearer token + JSON
// body). Keeping the implementation self-contained also lets the test
// suite assert on the exact polling cadence without negotiating with
// the api package's retry policy.
package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/build"
)

// EnvInsecureHTTP mirrors internal/api.EnvInsecureHTTP. Set to "1" to
// permit plaintext http:// against a development host. Production
// builds reject anything other than https://.
const EnvInsecureHTTP = "SHITHUB_DEV_INSECURE_HTTP"

// GrantType is the value of the `grant_type` form parameter for the
// access-token endpoint. RFC 8628 mandates this exact string.
const GrantType = "urn:ietf:params:oauth:grant-type:device_code"

// DefaultClientID is the public OAuth client identifier baked into the
// canonical shithub-cli binary. The server-side allowlist (S55) is
// expected to recognise it without a secret — device flow is for
// "public clients" per RFC 8628 §3.1.
const DefaultClientID = "shithub-cli"

// EnvClientIDOverride lets forks distribute their own client_id without
// rebuilding the binary. Empty / unset → DefaultClientID.
const EnvClientIDOverride = "SHITHUB_OAUTH_CLIENT_ID"

// Protocol error codes per RFC 8628 §3.5 + RFC 6749 §5.2. Server
// returns them as JSON {"error": "<code>"} bodies on HTTP 400.
var (
	ErrAuthorizationPending = errors.New("device: authorization_pending")
	ErrSlowDown             = errors.New("device: slow_down")
	ErrAccessDenied         = errors.New("device: access_denied")
	ErrExpiredToken         = errors.New("device: expired_token")
	ErrInvalidGrant         = errors.New("device: invalid_grant")
	ErrUnauthorizedClient   = errors.New("device: unauthorized_client")
	ErrUnsupportedGrantType = errors.New("device: unsupported_grant_type")
	ErrInvalidScope         = errors.New("device: invalid_scope")
	ErrInvalidRequest       = errors.New("device: invalid_request")
	ErrServerError          = errors.New("device: server_error")
)

// CodeResponse mirrors the JSON shape returned by POST /login/device/code.
// Field names match the wire contract; do not rename without coordinating
// with shithub/internal/web/handlers/auth/device_api.go.
type CodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenResponse mirrors the success body of POST /login/oauth/access_token.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// Scopes splits the comma-separated `scope` field into individual
// scope strings. Empty input yields a nil slice.
func (t TokenResponse) Scopes() []string {
	s := strings.TrimSpace(t.Scope)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Client carries the host + http.Client used for the three OAuth
// endpoints. Construct one per command invocation via NewClient.
type Client struct {
	host      string
	baseURL   string
	http      *http.Client
	userAgent string
	clientID  string
	clock     func() time.Time
	sleep     func(context.Context, time.Duration) error
}

// Options is the parameter object for NewClient.
type Options struct {
	// Host is the bare host (no scheme), e.g., "shithub.sh". Empty
	// defaults to "shithub.sh". Ignored when BaseURL is set.
	Host string
	// BaseURL overrides scheme + host. Useful for tests pointing at a
	// httptest.Server. When set, Host is ignored.
	BaseURL string
	// ClientID overrides the built-in public client identifier. Empty
	// falls back to EnvClientIDOverride, then DefaultClientID.
	ClientID string
	// HTTPClient overrides the transport. nil installs a fresh client
	// with a 30s timeout — same default as internal/api.
	HTTPClient *http.Client
	// UserAgent overrides the default `shithub-cli/<ver> (<os>/<arch>)`.
	UserAgent string
	// Sleep replaces the back-off between polls. Production leaves it
	// nil and uses a context-aware time.NewTimer wait; tests inject a
	// no-op (or instrumented) implementation to bypass real time.
	Sleep func(ctx context.Context, d time.Duration) error
}

// NewClient builds a Client. Returns an error if BaseURL is non-https
// without the EnvInsecureHTTP dev escape hatch.
func NewClient(opts Options) (*Client, error) {
	host := strings.TrimSpace(opts.Host)
	base := strings.TrimSpace(opts.BaseURL)
	if base == "" {
		if host == "" {
			host = "shithub.sh"
		}
		base = "https://" + host
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("device: parse base URL: %w", err)
	}
	if parsed.Scheme != "https" {
		if parsed.Scheme != "http" {
			return nil, fmt.Errorf("device: unsupported scheme %q", parsed.Scheme)
		}
		if os.Getenv(EnvInsecureHTTP) != "1" {
			return nil, fmt.Errorf("device: refusing http:// (%s); set %s=1 to override", parsed.String(), EnvInsecureHTTP)
		}
	}
	if host == "" {
		host = parsed.Host
	}

	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}

	ua := opts.UserAgent
	if ua == "" {
		ua = fmt.Sprintf("shithub-cli/%s (%s/%s)", build.Version, runtime.GOOS, runtime.GOARCH)
	}

	cid := strings.TrimSpace(opts.ClientID)
	if cid == "" {
		cid = strings.TrimSpace(os.Getenv(EnvClientIDOverride))
	}
	if cid == "" {
		cid = DefaultClientID
	}

	sleep := opts.Sleep
	if sleep == nil {
		sleep = contextSleep
	}

	return &Client{
		host:      host,
		baseURL:   strings.TrimRight(parsed.String(), "/"),
		http:      hc,
		userAgent: ua,
		clientID:  cid,
		clock:     time.Now,
		sleep:     sleep,
	}, nil
}

// ClientID returns the public OAuth client identifier this client will
// send. Exposed so callers can echo it back in a confirmation prompt.
func (c *Client) ClientID() string { return c.clientID }

// Host returns the bare host the client is bound to. Used to render
// post-login messages and verify-URI prefixes.
func (c *Client) Host() string { return c.host }

// RequestCode performs a single POST /login/device/code. scopes is a
// space- or comma-separated list of scopes; empty means "default set".
func (c *Client) RequestCode(ctx context.Context, scopes string) (*CodeResponse, error) {
	form := url.Values{}
	form.Set("client_id", c.clientID)
	if s := strings.TrimSpace(scopes); s != "" {
		form.Set("scope", s)
	}

	resp, err := c.postForm(ctx, "/login/device/code", form)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		var out CodeResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("device: decode code response: %w", err)
		}
		if out.DeviceCode == "" || out.UserCode == "" || out.VerificationURI == "" {
			return nil, fmt.Errorf("device: code response missing required fields")
		}
		if out.Interval <= 0 {
			out.Interval = 5
		}
		if out.ExpiresIn <= 0 {
			out.ExpiresIn = 900
		}
		return &out, nil
	}
	return nil, decodeOAuthError(resp)
}

// Exchange performs a single POST /login/oauth/access_token. Callers
// rarely invoke this directly — use Poll for the standard back-off
// loop. Returned errors are the package-level sentinels; the caller
// decides how to react (Poll handles pending/slow_down internally).
func (c *Client) Exchange(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", GrantType)

	resp, err := c.postForm(ctx, "/login/oauth/access_token", form)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		var out TokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("device: decode token response: %w", err)
		}
		if out.AccessToken == "" {
			return nil, fmt.Errorf("device: token response missing access_token")
		}
		if out.TokenType == "" {
			out.TokenType = "bearer"
		}
		return &out, nil
	}
	return nil, decodeOAuthError(resp)
}

// Poll loops on Exchange until the user approves, denies, or the grant
// expires. The starting interval comes from code.Interval; slow_down
// responses double it (RFC 8628 §3.5). expires_in caps the total wait.
// Returns the same sentinel errors as Exchange for terminal states.
func (c *Client) Poll(ctx context.Context, code *CodeResponse) (*TokenResponse, error) {
	if code == nil {
		return nil, errors.New("device: Poll called with nil CodeResponse")
	}
	interval := time.Duration(code.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := c.clock().Add(time.Duration(code.ExpiresIn) * time.Second)

	for {
		// Sleep first so we don't poll instantly the first iteration —
		// the user hasn't had time to read the screen yet.
		if err := c.sleep(ctx, interval); err != nil {
			return nil, err
		}
		if !c.clock().Before(deadline) {
			return nil, ErrExpiredToken
		}

		tok, err := c.Exchange(ctx, code.DeviceCode)
		if err == nil {
			return tok, nil
		}
		switch {
		case errors.Is(err, ErrAuthorizationPending):
			continue
		case errors.Is(err, ErrSlowDown):
			interval *= 2
			continue
		default:
			return nil, err
		}
	}
}

func (c *Client) postForm(ctx context.Context, path string, form url.Values) (*http.Response, error) {
	body := strings.NewReader(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("device: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device: %s: %w", path, err)
	}
	return resp, nil
}

// oauthErrorBody is the RFC 6749 §5.2 error shape.
type oauthErrorBody struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// decodeOAuthError maps the JSON `error` code to a package sentinel.
// Unknown codes return a wrapped ErrServerError so callers can detect
// "this is an OAuth-level failure" without hardcoding the string set.
func decodeOAuthError(resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)
	var body oauthErrorBody
	_ = json.Unmarshal(raw, &body)
	switch body.Error {
	case "authorization_pending":
		return ErrAuthorizationPending
	case "slow_down":
		return ErrSlowDown
	case "access_denied":
		return ErrAccessDenied
	case "expired_token":
		return ErrExpiredToken
	case "invalid_grant":
		return ErrInvalidGrant
	case "unauthorized_client":
		return ErrUnauthorizedClient
	case "unsupported_grant_type":
		return ErrUnsupportedGrantType
	case "invalid_scope":
		return ErrInvalidScope
	case "invalid_request":
		return ErrInvalidRequest
	case "server_error":
		return ErrServerError
	}
	if body.Error != "" {
		return fmt.Errorf("device: HTTP %d %s (%s): %w",
			resp.StatusCode, body.Error, body.ErrorDescription, ErrServerError)
	}
	snippet := strings.TrimSpace(string(raw))
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	return fmt.Errorf("device: HTTP %d: %s", resp.StatusCode, snippet)
}

// contextSleep waits for d or until ctx is cancelled, whichever comes
// first. Returned error matches ctx.Err() so callers can distinguish
// user-cancel from a clean timeout.
func contextSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
