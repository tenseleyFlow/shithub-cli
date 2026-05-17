// SPDX-License-Identifier: AGPL-3.0-or-later

package device

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient stands up an httptest.Server with the supplied handler
// and returns a Client pointed at it plus a controllable clock so
// tests can assert on polling behaviour without sleeping for real.
func newTestClient(t *testing.T, h http.Handler) (*Client, *fakeClock, func()) {
	t.Helper()
	t.Setenv(EnvInsecureHTTP, "1")
	srv := httptest.NewServer(h)
	c, err := NewClient(Options{BaseURL: srv.URL})
	if err != nil {
		srv.Close()
		t.Fatalf("NewClient: %v", err)
	}
	fc := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	c.clock = fc.Now
	c.sleep = fc.Sleep
	return c, fc, srv.Close
}

type fakeClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (f *fakeClock) Now() time.Time { return f.now }

func (f *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if d > 0 {
		f.sleeps = append(f.sleeps, d)
		f.now = f.now.Add(d)
	}
	return ctx.Err()
}

func TestRequestCodeHappyPath(t *testing.T) {
	var gotForm url.Values
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CodeResponse{
			DeviceCode:              "dc-123",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://example.test/login/device",
			VerificationURIComplete: "https://example.test/login/device?user_code=ABCD-EFGH",
			ExpiresIn:               900,
			Interval:                5,
		})
	}))
	defer cleanup()

	got, err := c.RequestCode(context.Background(), "repo:read user:read")
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if got.UserCode != "ABCD-EFGH" || got.DeviceCode != "dc-123" {
		t.Errorf("unexpected payload: %+v", got)
	}
	if gotForm.Get("client_id") != DefaultClientID {
		t.Errorf("client_id: %q, want %q", gotForm.Get("client_id"), DefaultClientID)
	}
	if gotForm.Get("scope") != "repo:read user:read" {
		t.Errorf("scope: %q", gotForm.Get("scope"))
	}
}

func TestRequestCodeOmitsEmptyScope(t *testing.T) {
	var gotForm url.Values
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(CodeResponse{
			DeviceCode:      "x",
			UserCode:        "YYYY-ZZZZ",
			VerificationURI: "https://example.test/login/device",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer cleanup()

	if _, err := c.RequestCode(context.Background(), "   "); err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	if _, present := gotForm["scope"]; present {
		t.Errorf("expected scope to be omitted, got: %v", gotForm["scope"])
	}
}

func TestRequestCodeUnauthorizedClient(t *testing.T) {
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unauthorized_client","error_description":"client_id not allowed"}`))
	}))
	defer cleanup()

	if _, err := c.RequestCode(context.Background(), ""); !errors.Is(err, ErrUnauthorizedClient) {
		t.Errorf("err = %v, want ErrUnauthorizedClient", err)
	}
}

func TestExchangeSuccess(t *testing.T) {
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != GrantType {
			http.Error(w, "wrong grant_type", http.StatusBadRequest)
			return
		}
		if r.PostForm.Get("device_code") != "dc-xyz" {
			http.Error(w, "wrong device_code", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "shithub_pat_abc",
			TokenType:   "bearer",
			Scope:       "user:read,repo:read",
		})
	}))
	defer cleanup()

	tok, err := c.Exchange(context.Background(), "dc-xyz")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "shithub_pat_abc" {
		t.Errorf("AccessToken: %q", tok.AccessToken)
	}
	if got := tok.Scopes(); len(got) != 2 || got[0] != "user:read" || got[1] != "repo:read" {
		t.Errorf("Scopes: %v", got)
	}
}

func TestPollPendingThenSuccess(t *testing.T) {
	var calls atomic.Int32
	c, fc, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "shithub_pat_ok",
			TokenType:   "bearer",
			Scope:       "user:read",
		})
	}))
	defer cleanup()

	tok, err := c.Poll(context.Background(), &CodeResponse{
		DeviceCode: "dc", UserCode: "AAAA-BBBB",
		VerificationURI: "https://example.test/login/device",
		ExpiresIn:       900, Interval: 5,
	})
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if tok.AccessToken != "shithub_pat_ok" {
		t.Errorf("AccessToken: %q", tok.AccessToken)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("server hit %d times, want 3", got)
	}
	if len(fc.sleeps) != 3 {
		t.Fatalf("sleeps: %v", fc.sleeps)
	}
	for i, d := range fc.sleeps {
		if d != 5*time.Second {
			t.Errorf("sleep[%d] = %s, want 5s", i, d)
		}
	}
}

func TestPollSlowDownDoublesInterval(t *testing.T) {
	var calls atomic.Int32
	c, fc, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		switch n {
		case 1:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		case 2:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"slow_down"}`))
		case 3:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
		default:
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "shithub_pat_x",
				TokenType:   "bearer",
				Scope:       "user:read",
			})
		}
	}))
	defer cleanup()

	_, err := c.Poll(context.Background(), &CodeResponse{
		DeviceCode: "dc", UserCode: "AAAA-BBBB",
		VerificationURI: "https://example.test/login/device",
		ExpiresIn:       900, Interval: 5,
	})
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(fc.sleeps) < 4 {
		t.Fatalf("expected >=4 sleeps, got %v", fc.sleeps)
	}
	// Interval sequence: 5s before call#1, 5s before call#2, 10s before
	// call#3 (slow_down doubled), 10s before call#4 (success).
	wants := []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second}
	for i, w := range wants {
		if fc.sleeps[i] != w {
			t.Errorf("sleep[%d] = %s, want %s", i, fc.sleeps[i], w)
		}
	}
}

func TestPollExpiredToken(t *testing.T) {
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"expired_token"}`))
	}))
	defer cleanup()

	_, err := c.Poll(context.Background(), &CodeResponse{
		DeviceCode: "dc", UserCode: "AAAA-BBBB",
		VerificationURI: "https://example.test/login/device",
		ExpiresIn:       60, Interval: 5,
	})
	if !errors.Is(err, ErrExpiredToken) {
		t.Errorf("err = %v, want ErrExpiredToken", err)
	}
}

func TestPollAccessDenied(t *testing.T) {
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer cleanup()

	_, err := c.Poll(context.Background(), &CodeResponse{
		DeviceCode: "dc", UserCode: "AAAA-BBBB",
		VerificationURI: "https://example.test/login/device",
		ExpiresIn:       900, Interval: 5,
	})
	if !errors.Is(err, ErrAccessDenied) {
		t.Errorf("err = %v, want ErrAccessDenied", err)
	}
}

func TestPollDeadlineExpiryAfterPending(t *testing.T) {
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer cleanup()

	// 12-second window with a 5s interval: sleep#1 advances to t+5
	// (still inside), sleep#2 to t+10 (still inside), sleep#3 to t+15
	// (past deadline). The deadline check runs after the sleep, so the
	// loop returns ErrExpiredToken before performing the third HTTP
	// call.
	_, err := c.Poll(context.Background(), &CodeResponse{
		DeviceCode: "dc", UserCode: "AAAA-BBBB",
		VerificationURI: "https://example.test/login/device",
		ExpiresIn:       12, Interval: 5,
	})
	if !errors.Is(err, ErrExpiredToken) {
		t.Errorf("err = %v, want ErrExpiredToken", err)
	}
}

func TestPollContextCancellation(t *testing.T) {
	c, _, cleanup := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	// Replace sleep with one that cancels then returns ctx.Err() on
	// first invocation. The Poll loop should surface that error.
	c.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := c.Poll(ctx, &CodeResponse{
		DeviceCode: "dc", UserCode: "AAAA-BBBB",
		VerificationURI: "https://example.test/login/device",
		ExpiresIn:       900, Interval: 5,
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestClientIDEnvOverride(t *testing.T) {
	t.Setenv(EnvClientIDOverride, "shithub-cli-fork")
	c, err := NewClient(Options{BaseURL: "https://example.test"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.ClientID() != "shithub-cli-fork" {
		t.Errorf("ClientID() = %q", c.ClientID())
	}
}

func TestNewClientRefusesPlainHTTP(t *testing.T) {
	t.Setenv(EnvInsecureHTTP, "")
	_, err := NewClient(Options{BaseURL: "http://localhost:8080"})
	if err == nil || !strings.Contains(err.Error(), "refusing http://") {
		t.Errorf("err = %v, want refusing http://", err)
	}
}

func TestNewClientAllowsInsecureHTTPWithOverride(t *testing.T) {
	t.Setenv(EnvInsecureHTTP, "1")
	if _, err := NewClient(Options{BaseURL: "http://localhost:8080"}); err != nil {
		t.Errorf("NewClient: %v", err)
	}
}

func TestTokenResponseScopesEmpty(t *testing.T) {
	if got := (TokenResponse{}).Scopes(); got != nil {
		t.Errorf("Scopes() = %v, want nil", got)
	}
	if got := (TokenResponse{Scope: " , ,"}).Scopes(); len(got) != 0 {
		t.Errorf("Scopes() = %v, want empty", got)
	}
}
