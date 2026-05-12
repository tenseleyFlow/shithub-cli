// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"errors"
	"math/rand/v2"
	"net/http"
	"time"
)

// retryBase is the first backoff interval. We start aggressive so
// transient failures recover fast and the cap prevents a runaway delay
// on a sustained outage.
const (
	retryBase    = 200 * time.Millisecond
	retryMax     = 5 * time.Second
	rateLimitCap = 60 * time.Second
)

// retryPolicy is the constants-as-struct used by Client; lifted out so
// tests can inject deterministic timing if we ever need to.
type retryPolicy struct {
	base       time.Duration
	cap        time.Duration
	rateMaxNap time.Duration
	maxRetries int
}

// defaultRetryPolicy returns the live configuration used by Client.
// MaxRetries comes from ClientOptions; the durations are package-private
// constants because there is no reason to expose them yet.
func defaultRetryPolicy(maxRetries int) retryPolicy {
	return retryPolicy{
		base:       retryBase,
		cap:        retryMax,
		rateMaxNap: rateLimitCap,
		maxRetries: maxRetries,
	}
}

// shouldRetry decides whether (method, response, error) warrants another
// attempt. Order of checks matters:
//
//  1. Hard transport errors (connection refused, dial timeout): retry on
//     idempotent methods only. The caller passes them as Go errors.
//  2. Rate-limit responses: retry with the parsed window when it fits in
//     rateMaxNap; surface RateLimitError otherwise.
//  3. 5xx responses: retry on idempotent methods always; on POST/PATCH
//     only when the body is reproducible (decided by Client.do, not here
//     — this function gets a hint via canReplay).
//  4. Everything else (2xx, 4xx): no retry. 4xx is a client error and
//     re-issuing won't help.
//
// Returns the nap duration when retry is in order, zero when not.
func (p retryPolicy) shouldRetry(method string, resp *http.Response, transportErr error, canReplay bool, attempt int) (time.Duration, bool) {
	if attempt >= p.maxRetries {
		return 0, false
	}

	if transportErr != nil {
		if isIdempotent(method) {
			return p.backoff(attempt), true
		}
		// Non-idempotent + transport error: only retry if the body is
		// replayable, e.g., a buffered []byte.
		if canReplay {
			return p.backoff(attempt), true
		}
		return 0, false
	}

	if resp == nil {
		return 0, false
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		// Rate-limit retry: wait until the reset, capped.
		_, retryAfter := parseRateLimit(resp.Header)
		if retryAfter <= 0 {
			retryAfter = p.backoff(attempt)
		}
		if retryAfter > p.rateMaxNap {
			return 0, false
		}
		return retryAfter, true
	}

	if resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented {
		if isIdempotent(method) || canReplay {
			return p.backoff(attempt), true
		}
		return 0, false
	}

	return 0, false
}

// backoff returns the wait duration for the given attempt index, applying
// exponential growth (base * 2^attempt) capped at p.cap, with jitter in
// [0, base) to avoid thundering-herd retries from many clients.
func (p retryPolicy) backoff(attempt int) time.Duration {
	d := p.base << attempt
	if d > p.cap || d < 0 { // guard against shift overflow
		d = p.cap
	}
	// Full jitter in [base/2, d]: provides decorrelation without being
	// so wide that small retries become arbitrarily long.
	jitter := time.Duration(rand.Int64N(int64(p.base)))
	return d/2 + jitter
}

// isIdempotent reports whether HTTP method is safe to retry on a transport
// error without risk of duplicating side effects.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	}
	return false
}

// ErrRetriesExhausted is returned by Client.do when every retry has been
// consumed without a definitive response or terminal error.
var ErrRetriesExhausted = errors.New("api: retries exhausted")
