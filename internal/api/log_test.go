// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURLStripsUserinfo(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{
			in:   "https://user:shithub_pat_abc@shithub.sh/owner/repo",
			want: "https://shithub.sh/owner/repo",
		},
		{
			in:   "https://shithub.sh/owner/repo",
			want: "https://shithub.sh/owner/repo",
		},
	}
	for _, tc := range cases {
		if got := redactURL(tc.in); got != tc.want {
			t.Errorf("redactURL(%q)\n  want %q\n  got  %q", tc.in, tc.want, got)
		}
	}
}

func TestRedactURLStripsTokenQuery(t *testing.T) {
	t.Parallel()
	in := "https://shithub.sh/api?token=shithub_pat_secret&keep=ok"
	got := redactURL(in)

	if strings.Contains(got, "shithub_pat_secret") {
		t.Errorf("token leaked in redacted URL: %q", got)
	}
	if !strings.Contains(got, "keep=ok") {
		t.Errorf("non-secret query param dropped: %q", got)
	}
	if !strings.Contains(got, "token=%5Bredacted%5D") && !strings.Contains(got, "token=[redacted]") {
		t.Errorf("token param not redacted: %q", got)
	}
}

func TestRedactURLPreservesNonURL(t *testing.T) {
	t.Parallel()
	// url.Parse is generous; verify we don't crash on garbage.
	if got := redactURL(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
}

func TestRedactHeader(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, value, want string
	}{
		{"Authorization", "Bearer shithub_pat_xxx", redactedHeaderValue},
		{"authorization", "token shithub_pat_xxx", redactedHeaderValue},
		{"Cookie", "session=abc", redactedHeaderValue},
		{"Proxy-Authorization", "Basic abc", redactedHeaderValue},
		{"X-API-Key", "abc", redactedHeaderValue},
		{"Content-Type", "application/json", "application/json"},
		{"X-Request-ID", "req-abc", "req-abc"},
	}
	for _, tc := range cases {
		if got := redactHeader(tc.name, tc.value); got != tc.want {
			t.Errorf("redactHeader(%q, ...): want %q got %q", tc.name, tc.want, got)
		}
	}
}

// TestRedactErrURLScrubsUserinfo covers the audit #137 fix: a transport
// error embedding `user:token@host` in its URL must not leak the
// userinfo through Error(). A nil input round-trips to nil; a non-URL
// error round-trips unchanged.
func TestRedactErrURLScrubsUserinfo(t *testing.T) {
	t.Parallel()

	if got := redactErrURL(nil); got != nil {
		t.Errorf("nil in -> got %v", got)
	}

	plain := errors.New("plain error")
	if got := redactErrURL(plain); got.Error() != "plain error" {
		t.Errorf("non-URL passthrough: %v", got)
	}

	wrapped := &url.Error{
		Op:  "Get",
		URL: "https://alice:supersecret@shithub.test/api/v1/user",
		Err: errors.New("dial tcp: timeout"),
	}
	got := redactErrURL(wrapped).Error()
	if strings.Contains(got, "supersecret") || strings.Contains(got, "alice:") {
		t.Errorf("userinfo leaked in error: %q", got)
	}
	if !strings.Contains(got, "shithub.test") {
		t.Errorf("host should still be visible for debugging: %q", got)
	}
}

func TestSilentLoggerWritesNothing(t *testing.T) {
	t.Parallel()
	l := newSilentLogger()
	// Default Discard writer; we just want to make sure no panic.
	l.Info("anything", "key", "value")
	l.Error("anything", "key", "value")
}
