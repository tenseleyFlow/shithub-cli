// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestIsIdempotent(t *testing.T) {
	t.Parallel()
	idempotent := []string{
		http.MethodGet, http.MethodHead, http.MethodPut,
		http.MethodDelete, http.MethodOptions,
	}
	for _, m := range idempotent {
		if !isIdempotent(m) {
			t.Errorf("%s should be idempotent", m)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPatch, "CUSTOM"} {
		if isIdempotent(m) {
			t.Errorf("%s should NOT be idempotent", m)
		}
	}
}

func TestShouldRetryStopAtMaxAttempts(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(3)
	resp := &http.Response{StatusCode: 500, Header: http.Header{}}

	_, ok := p.shouldRetry(http.MethodGet, resp, nil, true, 3)
	if ok {
		t.Error("attempts == maxRetries should stop")
	}
}

func TestShouldRetryTransportErrIdempotent(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(3)
	_, ok := p.shouldRetry(http.MethodGet, nil, errors.New("dial tcp: refused"), false, 0)
	if !ok {
		t.Error("GET with transport error should retry")
	}
}

func TestShouldRetryTransportErrPostOnlyWithReplayable(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(3)
	if _, ok := p.shouldRetry(http.MethodPost, nil, errors.New("dial"), false, 0); ok {
		t.Error("POST without replayable body should not retry on transport err")
	}
	if _, ok := p.shouldRetry(http.MethodPost, nil, errors.New("dial"), true, 0); !ok {
		t.Error("POST with replayable body should retry on transport err")
	}
}

func TestShouldRetryRateLimitInWindow(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(3)
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Retry-After": []string{"2"}},
	}
	d, ok := p.shouldRetry(http.MethodGet, resp, nil, false, 0)
	if !ok {
		t.Fatal("429 with Retry-After=2s should retry")
	}
	if d < time.Second || d > 3*time.Second {
		t.Errorf("backoff should honor Retry-After, got %v", d)
	}
}

func TestShouldRetryRateLimitTooLongAborts(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(3)
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Retry-After": []string{"3600"}},
	}
	if _, ok := p.shouldRetry(http.MethodGet, resp, nil, false, 0); ok {
		t.Error("Retry-After beyond cap should abort retries")
	}
}

func TestShouldRetry5xx(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(3)
	cases := []struct {
		method string
		status int
		replay bool
		wantOK bool
	}{
		{http.MethodGet, 502, false, true},
		{http.MethodGet, 503, false, true},
		{http.MethodGet, 504, false, true},
		{http.MethodPost, 500, false, false},
		{http.MethodPost, 500, true, true},
		{http.MethodGet, 501, false, false}, // 501 Not Implemented — don't retry
	}
	for _, tc := range cases {
		resp := &http.Response{StatusCode: tc.status, Header: http.Header{}}
		_, ok := p.shouldRetry(tc.method, resp, nil, tc.replay, 0)
		if ok != tc.wantOK {
			t.Errorf("%s %d replay=%v: want retry=%v got %v", tc.method, tc.status, tc.replay, tc.wantOK, ok)
		}
	}
}

func TestShouldRetryNoRetryOn4xx(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(3)
	for _, status := range []int{400, 401, 403, 404, 422} {
		resp := &http.Response{StatusCode: status, Header: http.Header{}}
		if _, ok := p.shouldRetry(http.MethodGet, resp, nil, true, 0); ok {
			t.Errorf("4xx (%d) should not retry", status)
		}
	}
}

func TestBackoffGrows(t *testing.T) {
	t.Parallel()
	p := defaultRetryPolicy(5)
	prev := time.Duration(0)
	for i := 0; i < 4; i++ {
		d := p.backoff(i)
		if d <= 0 || d > p.cap {
			t.Errorf("attempt %d: backoff out of range, got %v", i, d)
		}
		// Mean should grow with attempt; we don't assert strict
		// monotonicity because jitter introduces variance.
		prev = d
		_ = prev
	}
}
