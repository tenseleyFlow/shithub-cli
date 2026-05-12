// SPDX-License-Identifier: AGPL-3.0-or-later

// Package api is shithub-cli's HTTP client to shithub. Every authenticated
// command builds one Client per invocation and routes all server traffic
// through it: auth header injection, retry, pagination, error decoding,
// and request-id propagation live here. No package under pkg/cmd/ should
// import net/http directly — go through Client.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// errorEnvelope is the shape shithub returns for 4xx/5xx errors. Locking
// this contract in S50 §0 (cross-cutting): every /api/v1 error responds
// with `{"error": "<human-readable>"}`. If shithub ever adds fields here
// (e.g., a structured `code`), extend this type — never decode ad hoc at
// call sites.
type errorEnvelope struct {
	Error string `json:"error"`
}

// APIError is the catch-all for server-side failures the client could not
// classify into a more specific typed error. Holds status, the decoded
// message, the request-id (for support reports), and the raw body (for
// debugging when --verbose is on).
type APIError struct {
	StatusCode int
	Message    string
	RequestID  string
	RawBody    []byte
}

// Error returns a user-facing summary suitable for stderr printing.
// Format favors operator clarity over machine parsing — for that, callers
// should errors.As against the typed error and read fields directly.
func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("shithub API: %d %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("shithub API: %d (no message)", e.StatusCode)
}

// AuthError reports a 401 (unauthenticated, expired, or revoked token).
// Wraps APIError so callers who don't care about the distinction can
// errors.As against *APIError uniformly.
type AuthError struct {
	APIError
	WWWAuthenticate string // raw WWW-Authenticate header value, if any
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("shithub: not authenticated (%d). Run `shithub auth login`.", e.StatusCode)
}

// Unwrap exposes the underlying APIError so errors.As(err, &api.APIError{}) works.
func (e *AuthError) Unwrap() error { return &e.APIError }

// ScopeError reports a 403 caused by a token-scope mismatch. shithub
// formats the message as "token lacks required scope: repo:write"
// (S08 contract). The constructor parses that out so callers can render
// a helpful "run `shithub auth refresh -s repo:write`" hint.
type ScopeError struct {
	APIError
	RequiredScope  string
	ProvidedScopes []string
}

func (e *ScopeError) Error() string {
	if e.RequiredScope != "" {
		return fmt.Sprintf("shithub: token lacks scope %q. Run `shithub auth refresh -s %s`.", e.RequiredScope, e.RequiredScope)
	}
	return fmt.Sprintf("shithub: permission denied (%d): %s", e.StatusCode, e.Message)
}

// Unwrap exposes the underlying APIError.
func (e *ScopeError) Unwrap() error { return &e.APIError }

// NotFoundError reports a 404. May indicate either a genuinely missing
// resource OR an existence-leak-safe denial (shithub returns 404 for
// private repos when the viewer lacks access — that's deliberate, not a
// bug). Commands typically translate to a friendly "could not find X".
type NotFoundError struct {
	APIError
}

func (e *NotFoundError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("shithub: not found: %s", e.Message)
	}
	return "shithub: not found"
}

// Unwrap exposes the underlying APIError.
func (e *NotFoundError) Unwrap() error { return &e.APIError }

// RateLimitError reports a 429. ResetAt carries the parsed
// X-RateLimit-Reset / Retry-After value when present so callers (or the
// retry policy) can sleep until the window reopens.
type RateLimitError struct {
	APIError
	ResetAt    time.Time
	RetryAfter time.Duration // raw Retry-After interpretation; zero when absent
}

func (e *RateLimitError) Error() string {
	if !e.ResetAt.IsZero() {
		return fmt.Sprintf("shithub: rate limited; resets at %s", e.ResetAt.Format(time.RFC3339))
	}
	return "shithub: rate limited"
}

// Unwrap exposes the underlying APIError.
func (e *RateLimitError) Unwrap() error { return &e.APIError }

// classifyResponse builds the right typed error for the given HTTP
// response status + body. The body is consumed by the caller before this
// function is invoked; we just pick the type, decode the message, and
// stash request-id + raw bytes for surface-level diagnostics.
//
// Status codes outside the well-known set (401/403/404/429) fall back to
// a plain *APIError. 2xx never reaches here.
func classifyResponse(resp *http.Response, body []byte) error {
	msg := parseErrorMessage(body)
	requestID := resp.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = resp.Header.Get("X-Request-ID")
	}

	base := APIError{
		StatusCode: resp.StatusCode,
		Message:    msg,
		RequestID:  requestID,
		RawBody:    body,
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return &AuthError{
			APIError:        base,
			WWWAuthenticate: resp.Header.Get("WWW-Authenticate"),
		}
	case http.StatusForbidden:
		required, provided := parseScopeMessage(msg, resp.Header)
		return &ScopeError{
			APIError:       base,
			RequiredScope:  required,
			ProvidedScopes: provided,
		}
	case http.StatusNotFound:
		return &NotFoundError{APIError: base}
	case http.StatusTooManyRequests:
		resetAt, retryAfter := parseRateLimit(resp.Header)
		return &RateLimitError{
			APIError:   base,
			ResetAt:    resetAt,
			RetryAfter: retryAfter,
		}
	}
	return &base
}

// parseErrorMessage extracts the "error" field from a JSON envelope.
// Returns the empty string when the body is not JSON (or doesn't have
// the field); callers fall back to "%d (no message)".
func parseErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return strings.TrimSpace(env.Error)
}

// parseScopeMessage pulls a required-scope name out of a server message
// of the form "token lacks required scope: <scope>" (S08 contract). Also
// reads the X-OAuth-Scopes header (S50 §1) to populate the provided list,
// so commands can show "you have repo:read, need repo:write" diagnostics.
func parseScopeMessage(msg string, header http.Header) (required string, provided []string) {
	const prefix = "token lacks required scope:"
	if idx := strings.Index(msg, prefix); idx >= 0 {
		required = strings.TrimSpace(msg[idx+len(prefix):])
		// Some servers may add additional trailing context; clip at first space.
		if cut := strings.IndexByte(required, ' '); cut > 0 {
			required = required[:cut]
		}
	}

	if scopes := header.Get("X-OAuth-Scopes"); scopes != "" {
		for _, s := range strings.Split(scopes, ",") {
			if s = strings.TrimSpace(s); s != "" {
				provided = append(provided, s)
			}
		}
	}
	return required, provided
}

// parseRateLimit reads X-RateLimit-Reset (unix-seconds) and Retry-After
// (seconds or HTTP-date) and returns the absolute reset time plus the
// raw Retry-After interpretation. Retry-After wins when both are present
// because some servers stamp X-RateLimit-Reset on every response.
func parseRateLimit(header http.Header) (time.Time, time.Duration) {
	if v := header.Get("Retry-After"); v != "" {
		// Seconds form.
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			retryAfter := time.Duration(secs) * time.Second
			return time.Now().Add(retryAfter), retryAfter
		}
		// HTTP-date form.
		if t, err := http.ParseTime(v); err == nil {
			return t, time.Until(t)
		}
	}
	if v := header.Get("X-RateLimit-Reset"); v != "" {
		if secs, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(secs, 0), 0
		}
	}
	return time.Time{}, 0
}

// IsAuthError reports whether err is (or wraps) an AuthError. Convenience
// wrapper that hides the errors.As ceremony at call sites.
func IsAuthError(err error) bool {
	var t *AuthError
	return errors.As(err, &t)
}

// IsScopeError reports whether err is (or wraps) a ScopeError.
func IsScopeError(err error) bool {
	var t *ScopeError
	return errors.As(err, &t)
}

// IsNotFoundError reports whether err is (or wraps) a NotFoundError.
func IsNotFoundError(err error) bool {
	var t *NotFoundError
	return errors.As(err, &t)
}

// IsRateLimitError reports whether err is (or wraps) a RateLimitError.
func IsRateLimitError(err error) bool {
	var t *RateLimitError
	return errors.As(err, &t)
}
