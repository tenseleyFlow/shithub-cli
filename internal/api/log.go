// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"errors"
	"log/slog"
	"net/url"
	"strings"
)

// EnvDebug is the on-switch for client-level slog output. Any non-empty
// value enables; we deliberately do not parse booleans so users typing
// `SHITHUB_DEBUG=1 shithub api ...` get verbose mode without remembering
// truthy spellings.
const EnvDebug = "SHITHUB_DEBUG"

// redactedHeaderValue is the placeholder we emit anywhere an Authorization
// header would otherwise appear in logs or error messages.
const redactedHeaderValue = "[redacted]"

// redactURL strips embedded credentials from a URL so it's safe to log.
// Input may be a bare URL string or anything else; non-URL input is
// returned unchanged.
//
// Patterns scrubbed:
//   - userinfo: scheme://user:token@host/path -> scheme://host/path
//   - URL query values matching common token names (token, access_token,
//     oauth_token) replaced with [redacted]
func redactURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User != nil {
		u.User = nil
	}
	if q := u.Query(); len(q) > 0 {
		changed := false
		for _, key := range tokenQueryKeys {
			if q.Get(key) != "" {
				q.Set(key, redactedHeaderValue)
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}
	return u.String()
}

// tokenQueryKeys is the set of query parameter names we treat as
// secret-bearing. Adding to this list is cheap; the alternative
// (redacting every query parameter) is too noisy.
var tokenQueryKeys = []string{"token", "access_token", "oauth_token", "auth"}

// redactErrURL walks the wrapped chain of err looking for *url.Error
// (the canonical net/http transport-error type) and returns a fresh
// error whose URL field is scrubbed by redactURL. Returns err unchanged
// when the chain doesn't carry one. This is the safety net for the
// RESTRaw transport-error path, which would otherwise propagate a raw
// "https://user:token@host: dial tcp ..." string straight to stderr.
func redactErrURL(err error) error {
	if err == nil {
		return nil
	}
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	// Build a copy with the URL string scrubbed. We can't mutate ue in
	// place safely (it may be shared via errors.As semantics), so we
	// return a wrapper that prints the scrubbed form while preserving
	// the unwrap chain for callers that inspect typed errors.
	return &redactedURLError{op: ue.Op, url: redactURL(ue.URL), err: ue.Err}
}

// redactedURLError mirrors url.Error's shape but renders a scrubbed
// URL. The Err field stays unwrappable so errors.As / errors.Is on the
// underlying network error keeps working.
type redactedURLError struct {
	op  string
	url string
	err error
}

func (e *redactedURLError) Error() string {
	return e.op + " " + e.url + ": " + e.err.Error()
}

func (e *redactedURLError) Unwrap() error { return e.err }

// redactHeader returns a value safe to log for the given header. Returns
// "[redacted]" for any name we treat as secret-bearing; returns the
// original value otherwise.
func redactHeader(name, value string) string {
	if isSensitiveHeader(name) {
		return redactedHeaderValue
	}
	return value
}

// isSensitiveHeader reports whether the named header should never appear
// in log output. Match is case-insensitive (HTTP headers are).
func isSensitiveHeader(name string) bool {
	n := strings.ToLower(name)
	for _, s := range sensitiveHeaders {
		if n == s {
			return true
		}
	}
	return false
}

var sensitiveHeaders = []string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
	"x-auth-token",
}

// newSilentLogger returns a slog.Logger that discards everything. Used as
// the client default so production binaries don't leak debug noise.
func newSilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
}

// discardWriter satisfies io.Writer over /dev/null without an actual file.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
