// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// mkResp builds a fake response for classifyResponse exercise. The body
// is http.NoBody (no real resource to close), so bodyclose suppression
// at the package level (see .golangci.yml) is safe.
func mkResp(status int, _ string, headers map[string]string) *http.Response {
	r := &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       http.NoBody,
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestClassify401EmitsAuthError(t *testing.T) {
	t.Parallel()
	resp := mkResp(401, "", map[string]string{
		"WWW-Authenticate": `Bearer realm="shithub" error="invalid token"`,
		"X-Request-Id":     "req-abc",
	})
	err := classifyResponse(resp, []byte(`{"error":"bad token"}`))

	if !IsAuthError(err) {
		t.Fatalf("want AuthError, got %T: %v", err, err)
	}
	var authErr *AuthError
	if !errors.As(err, &authErr) {
		t.Fatal("errors.As AuthError failed")
	}
	if authErr.WWWAuthenticate == "" {
		t.Error("WWW-Authenticate not preserved")
	}
	if authErr.RequestID != "req-abc" {
		t.Errorf("RequestID: want req-abc got %q", authErr.RequestID)
	}
	if !strings.Contains(authErr.Error(), "auth login") {
		t.Errorf("AuthError message should hint at auth login, got %q", authErr.Error())
	}

	// Unwrap chain reaches APIError.
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Error("AuthError should unwrap to *APIError")
	}
	if apiErr.Message != "bad token" {
		t.Errorf("APIError.Message: got %q", apiErr.Message)
	}
}

func TestClassify403EmitsScopeError(t *testing.T) {
	t.Parallel()
	resp := mkResp(403, "", map[string]string{
		"X-OAuth-Scopes": "repo:read, user:read",
	})
	body := []byte(`{"error":"token lacks required scope: repo:write"}`)
	err := classifyResponse(resp, body)

	if !IsScopeError(err) {
		t.Fatalf("want ScopeError, got %T: %v", err, err)
	}
	var scopeErr *ScopeError
	_ = errors.As(err, &scopeErr)
	if scopeErr.RequiredScope != "repo:write" {
		t.Errorf("RequiredScope: got %q", scopeErr.RequiredScope)
	}
	if len(scopeErr.ProvidedScopes) != 2 || scopeErr.ProvidedScopes[0] != "repo:read" {
		t.Errorf("ProvidedScopes: got %v", scopeErr.ProvidedScopes)
	}
	if !strings.Contains(scopeErr.Error(), "auth refresh -s repo:write") {
		t.Errorf("ScopeError should suggest refresh, got %q", scopeErr.Error())
	}
}

func TestClassify404EmitsNotFoundError(t *testing.T) {
	t.Parallel()
	resp := mkResp(404, "", nil)
	err := classifyResponse(resp, []byte(`{"error":"repo not found"}`))

	if !IsNotFoundError(err) {
		t.Fatalf("want NotFoundError, got %T: %v", err, err)
	}
	var nfe *NotFoundError
	_ = errors.As(err, &nfe)
	if !strings.Contains(nfe.Error(), "repo not found") {
		t.Errorf("NotFoundError.Error: %q", nfe.Error())
	}
}

func TestClassify429EmitsRateLimitWithRetryAfter(t *testing.T) {
	t.Parallel()
	resp := mkResp(429, "", map[string]string{"Retry-After": "30"})
	err := classifyResponse(resp, nil)

	if !IsRateLimitError(err) {
		t.Fatalf("want RateLimitError, got %T", err)
	}
	var rle *RateLimitError
	_ = errors.As(err, &rle)
	if rle.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter: want 30s got %v", rle.RetryAfter)
	}
	if rle.ResetAt.IsZero() {
		t.Error("ResetAt should be populated when Retry-After present")
	}
}

func TestClassify429EmitsRateLimitFromXRateLimitReset(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(2 * time.Minute).Unix()
	resp := mkResp(429, "", map[string]string{"X-RateLimit-Reset": strconv.FormatInt(future, 10)})
	err := classifyResponse(resp, nil)

	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Fatalf("want RateLimitError")
	}
	if rle.ResetAt.Unix() != future {
		t.Errorf("ResetAt: want %d got %d", future, rle.ResetAt.Unix())
	}
}

func TestClassify500EmitsBareAPIError(t *testing.T) {
	t.Parallel()
	resp := mkResp(500, "", nil)
	err := classifyResponse(resp, []byte(`{"error":"internal"}`))

	if IsAuthError(err) || IsScopeError(err) || IsNotFoundError(err) || IsRateLimitError(err) {
		t.Fatalf("500 should be plain APIError, got %T", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("StatusCode: got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "internal" {
		t.Errorf("Message: got %q", apiErr.Message)
	}
}

func TestParseErrorMessageNonJSONReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := parseErrorMessage([]byte("<html>500</html>")); got != "" {
		t.Errorf("non-JSON body should yield empty msg, got %q", got)
	}
	if got := parseErrorMessage(nil); got != "" {
		t.Errorf("nil body should yield empty msg, got %q", got)
	}
}

// G8c (F7/F52): the typed error .Error() outputs must NOT carry the
// "shithub: " prefix — root.go's stderr printer already prepends it.
// Pre-fix every printed error was `shithub: shithub: ...`. Pins all
// four typed errors against the doubled-prefix regression.
func TestTypedErrorsDoNotDoublePrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
	}{
		{"auth", classifyResponse(mkResp(401, "", nil), nil)},
		{"scope", classifyResponse(mkResp(403, "", nil), []byte(`{"error":"token lacks required scope: repo:write"}`))},
		{"not-found-empty", classifyResponse(mkResp(404, "", nil), nil)},
		{"not-found-with-msg", classifyResponse(mkResp(404, "", nil), []byte(`{"error":"repo not found"}`))},
		{"rate-limit", classifyResponse(mkResp(429, "", nil), nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if strings.HasPrefix(tc.err.Error(), "shithub:") {
				t.Errorf("%s: %q starts with 'shithub:' — root.go will double-prefix", tc.name, tc.err.Error())
			}
		})
	}
}

// G8c (F7): when the server message already contains "not found",
// NotFoundError must not prepend a second copy. Pre-fix the printed
// error was `shithub: shithub: not found: pull request not found`
// — three different "not found" / "shithub" tokens stuttering.
func TestNotFoundErrorAvoidsDoubledNoun(t *testing.T) {
	t.Parallel()
	err := classifyResponse(mkResp(404, "", nil), []byte(`{"error":"pull request not found"}`))
	var nfe *NotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("want NotFoundError, got %T", err)
	}
	// The message itself contains "not found"; the error string
	// should be exactly the server message, no "not found: " prefix.
	if got := nfe.Error(); got != "pull request not found" {
		t.Errorf("doubled-noun guard: got %q want %q", got, "pull request not found")
	}
}
