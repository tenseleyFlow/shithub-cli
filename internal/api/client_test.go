// SPDX-License-Identifier: AGPL-3.0-or-later

package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

// Helpers ----------------------------------------------------------------

// asJSON marshals v or fails the test; trims test boilerplate.
func asJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// REST behaviour ---------------------------------------------------------

// TestOwnerRepoPlaceholdersEscaped verifies that a hostile owner/repo
// value is %-encoded before substitution. Without escaping, the bytes
// "../../admin" would substitute literally and the request would target
// /api/v1/repos/../../admin/r/contents — server-side path-cleaning
// could then route it to /api/v1/admin/... and bypass the resource
// scoping the {owner}/{repo} placeholders were meant to enforce.
//
// We use an httptest server directly (rather than fakeapi's routing)
// so we can read r.RequestURI, which preserves the original wire bytes
// before Go's net/http decodes path-escapes.
func TestOwnerRepoPlaceholdersEscaped(t *testing.T) {
	t.Setenv(api.EnvInsecureHTTP, "1")
	var rawURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawURI = r.RequestURI
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	c, err := api.NewClient(api.ClientOptions{
		BaseURL:   srv.URL,
		TokenFunc: func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
		Timeout:   2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_ = c.REST(context.Background(), "GET", "/repos/{owner}/{repo}/contents", nil, nil,
		api.WithOwner("../../admin"), api.WithRepo("r"))
	if !strings.Contains(rawURI, "%2F") {
		t.Errorf("expected raw URI to contain percent-encoded slashes, got %q", rawURI)
	}
	if strings.Contains(rawURI, "/../") {
		t.Errorf("raw URI should NOT contain literal /../, got %q", rawURI)
	}
}

func TestRESTGetUnmarshalsBody(t *testing.T) {
	fake := fakeapi.New(t)
	fake.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"username": "mf"})
	c := fake.NewClient()

	var got struct {
		Username string `json:"username"`
	}
	if err := c.REST(context.Background(), "GET", "user", nil, &got); err != nil {
		t.Fatalf("REST: %v", err)
	}
	if got.Username != "mf" {
		t.Errorf("username: got %q", got.Username)
	}
}

func TestRESTPostBodyShape(t *testing.T) {
	fake := fakeapi.New(t)
	fake.Handle("POST", "/api/v1/repos", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["name"] != "shithub-cli" {
			t.Errorf("body.name: got %v", payload["name"])
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type: got %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":42}`))
	})

	c := fake.NewClient()
	var got struct {
		ID int `json:"id"`
	}
	if err := c.REST(context.Background(), "POST", "repos",
		map[string]any{"name": "shithub-cli"}, &got); err != nil {
		t.Fatalf("REST: %v", err)
	}
	if got.ID != 42 {
		t.Errorf("id: got %d", got.ID)
	}
}

func TestRESTAuthErrorOn401(t *testing.T) {
	fake := fakeapi.New(t)
	fake.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="shithub"`)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	})

	c := fake.NewClient()
	var dst struct{}
	err := c.REST(context.Background(), "GET", "user", nil, &dst)
	if !api.IsAuthError(err) {
		t.Fatalf("expected AuthError, got %v", err)
	}
}

func TestRESTScopeErrorOn403(t *testing.T) {
	fake := fakeapi.New(t)
	fake.Handle("POST", "/api/v1/repos", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo:read")
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"token lacks required scope: repo:write"}`))
	})

	c := fake.NewClient()
	err := c.REST(context.Background(), "POST", "repos", map[string]any{}, nil)
	if !api.IsScopeError(err) {
		t.Fatalf("expected ScopeError, got %v", err)
	}
}

func TestRESTRetryOn5xx(t *testing.T) {
	var hits int32
	fake := fakeapi.New(t)
	fake.Handle("GET", "/api/v1/flaky", func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(502)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	c, err := api.NewClient(api.ClientOptions{
		BaseURL:    fake.URL(),
		TokenFunc:  func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
		MaxRetries: api.IntPtr(4),
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	var got map[string]any
	if err := c.REST(context.Background(), "GET", "flaky", nil, &got); err != nil {
		t.Fatalf("REST after retries: %v", err)
	}
	if hits := atomic.LoadInt32(&hits); hits != 3 {
		t.Errorf("expected 3 server hits (2 retries), got %d", hits)
	}
}

// TestMaxRetriesZeroDisablesRetries verifies that explicit zero (via
// IntPtr(0)) actually disables retries — distinguishing the explicit
// "disable" intent from the Go zero-value "unset" case. C03 DoD test.
func TestMaxRetriesZeroDisablesRetries(t *testing.T) {
	var hits int32
	fake := fakeapi.New(t)
	fake.Handle("GET", "/api/v1/flaky", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(502)
	})

	c, err := api.NewClient(api.ClientOptions{
		BaseURL:    fake.URL(),
		TokenFunc:  func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
		MaxRetries: api.IntPtr(0),
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.REST(context.Background(), "GET", "flaky", nil, nil); err == nil {
		t.Fatal("expected error on 502 with retries disabled")
	}
	if hits := atomic.LoadInt32(&hits); hits != 1 {
		t.Errorf("expected 1 server hit (no retries), got %d", hits)
	}
}

// TestMaxRetriesNilUsesDefault confirms nil leaves DefaultMaxRetries in
// place — guarding against the prior bug where Go-zero ate the explicit
// disable intent.
func TestMaxRetriesNilUsesDefault(t *testing.T) {
	var hits int32
	fake := fakeapi.New(t)
	fake.Handle("GET", "/api/v1/flaky", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(502)
	})

	c, err := api.NewClient(api.ClientOptions{
		BaseURL:   fake.URL(),
		TokenFunc: func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
		// MaxRetries omitted -> should be DefaultMaxRetries (3).
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_ = c.REST(context.Background(), "GET", "flaky", nil, nil)
	// 1 initial + DefaultMaxRetries retries = 4 attempts total.
	if hits := atomic.LoadInt32(&hits); hits != int32(api.DefaultMaxRetries+1) {
		t.Errorf("expected %d hits, got %d", api.DefaultMaxRetries+1, hits)
	}
}

func TestRESTNoRetryOn4xx(t *testing.T) {
	var hits int32
	fake := fakeapi.New(t)
	fake.Handle("GET", "/api/v1/cold", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"error":"unprocessable"}`))
	})

	c := fake.NewClient()
	err := c.REST(context.Background(), "GET", "cold", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("4xx should not retry; got %d hits", got)
	}
}

// Headers ----------------------------------------------------------------

func TestRequestHeadersInclude(t *testing.T) {
	fake := fakeapi.New(t)
	fake.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{})
	c := fake.NewClient()

	if err := c.REST(context.Background(), "GET", "user", nil, nil); err != nil {
		t.Fatalf("REST: %v", err)
	}

	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	h := calls[0].Header
	if !strings.HasPrefix(h.Get("Authorization"), "token ") {
		t.Errorf("Authorization header malformed: %q", h.Get("Authorization"))
	}
	if h.Get("Accept") != "application/json" {
		t.Errorf("Accept: got %q", h.Get("Accept"))
	}
	if h.Get("X-Request-Id") == "" {
		t.Error("X-Request-Id not set")
	}
	if !strings.HasPrefix(h.Get("User-Agent"), "shithub-cli/") {
		t.Errorf("User-Agent: got %q", h.Get("User-Agent"))
	}
}

// Path templating --------------------------------------------------------

func TestPathPlaceholderSubstitution(t *testing.T) {
	fake := fakeapi.New(t)
	fake.RegisterJSON("GET", "/api/v1/repos/owner/repo/issues", 200, []any{})
	c := fake.NewClient()

	if err := c.REST(context.Background(), "GET", "repos/{owner}/{repo}/issues", nil, nil,
		api.WithOwner("owner"), api.WithRepo("repo")); err != nil {
		t.Fatalf("REST: %v", err)
	}
	fake.AssertCalled("GET", "/api/v1/repos/owner/repo/issues")
}

func TestPathAbsolutePathPassesThrough(t *testing.T) {
	fake := fakeapi.New(t)
	fake.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{})
	c := fake.NewClient()

	// Caller passes the fully qualified API path; no double-prefix.
	if err := c.REST(context.Background(), "GET", "/api/v1/user", nil, nil); err != nil {
		t.Fatalf("REST: %v", err)
	}
	fake.AssertCalled("GET", "/api/v1/user")
}

// Pagination -------------------------------------------------------------

func TestDoPaginatedFollowsLink(t *testing.T) {
	fake := fakeapi.New(t)

	// Page 1 advertises page 2; page 2 stops.
	fake.Handle("GET", "/api/v1/items", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/items?page=2>; rel="next"`, fake.URL()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(asJSON(t, []map[string]any{{"id": 1}, {"id": 2}}))
	})
	fake.Handle("GET", "/api/v1/items?page=2", func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("page=2 handler should not fire — Go pattern match excludes query")
	})
	// Path-only handler for page 2 (the dispatcher strips query when matching).
	fake.Handle("GET", "/api/v1/items", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "page=2" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write(asJSON(t, []map[string]any{{"id": 3}}))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/items?page=2>; rel="next"`, fake.URL()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(asJSON(t, []map[string]any{{"id": 1}, {"id": 2}}))
	})

	c := fake.NewClient()
	var pages [][]map[string]any
	for raw, err := range c.DoPaginated(context.Background(), "GET", "items") {
		if err != nil {
			t.Fatalf("paginated: %v", err)
		}
		var batch []map[string]any
		if err := json.Unmarshal(raw, &batch); err != nil {
			t.Fatalf("unmarshal page: %v", err)
		}
		pages = append(pages, batch)
	}

	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if len(pages[0]) != 2 || len(pages[1]) != 1 {
		t.Errorf("page sizes: %v", pages)
	}
}

func TestDoPaginatedRespectsMaxPages(t *testing.T) {
	fake := fakeapi.New(t)

	// Every call advertises a next page; the cap stops us.
	fake.Handle("GET", "/api/v1/loop", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/loop?page=next>; rel="next"`, fake.URL()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("[]"))
	})

	c := fake.NewClient()
	count := 0
	for _, err := range c.DoPaginated(context.Background(), "GET", "loop", api.WithMaxPages(2)) {
		if err != nil {
			t.Fatalf("paginated: %v", err)
		}
		count++
	}
	if count != 2 {
		t.Errorf("WithMaxPages(2) should yield 2 pages, got %d", count)
	}
}

// TestDoPaginatedRetriesOnLaterPages covers the audit #134 fix: a
// transient 5xx on page 2 (or any subsequent page) must be retried, not
// surface as a walk-terminating error. The earlier impl routed page 1
// through RESTRaw (with retries) but pages 2+ through doRaw (no
// retries), failing the whole walk on the first hiccup mid-stream.
func TestDoPaginatedRetriesOnLaterPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.RawQuery {
		case "":
			// Page 1: succeed, advertise page 2.
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?p=2>; rel="next"`, "http://"+r.Host, r.URL.Path))
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`[{"id":1}]`))
		case "p=2":
			// Page 2: succeed, advertise page 3.
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?p=3>; rel="next"`, "http://"+r.Host, r.URL.Path))
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`[{"id":2}]`))
		case "p=3":
			// Page 3 is flaky: first attempt fails 502, second succeeds.
			if hits.flaky.Add(1) < 2 {
				w.WriteHeader(502)
				return
			}
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`[{"id":3}]`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv(api.EnvInsecureHTTP, "1")

	c, err := api.NewClient(api.ClientOptions{
		BaseURL:    srv.URL,
		TokenFunc:  func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
		MaxRetries: api.IntPtr(2),
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	var ids []int
	for raw, err := range c.DoPaginated(context.Background(), "GET", "/things") {
		if err != nil {
			t.Fatalf("DoPaginated: %v", err)
		}
		var batch []map[string]int
		_ = json.Unmarshal(raw, &batch)
		for _, b := range batch {
			ids = append(ids, b["id"])
		}
	}
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Errorf("expected ids 1,2,3 across 3 pages with retry on page 3; got %v", ids)
	}
}

// hits is shared counter scaffolding for TestDoPaginatedRetriesOnLaterPages
// — defined at package scope so the handler closure can increment it
// without juggling a wrapper struct.
var hits struct {
	flaky atomic.Int32
}

// Insecure-http guard ----------------------------------------------------

func TestRejectsHTTPWithoutEscape(t *testing.T) {
	// We do NOT call fakeapi.New (which flips the env on) — instead build a
	// raw httptest server and explicitly leave the env off.
	t.Setenv(api.EnvInsecureHTTP, "")
	_, err := api.NewClient(api.ClientOptions{
		BaseURL:   "http://shithub.test",
		TokenFunc: func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
	})
	if err == nil {
		t.Fatal("expected error for http:// without escape hatch")
	}
	if !strings.Contains(err.Error(), api.EnvInsecureHTTP) {
		t.Errorf("error should mention the escape env var, got: %v", err)
	}
}

// TokenFunc plumbing -----------------------------------------------------

func TestTokenFuncErrorPropagates(t *testing.T) {
	fake := fakeapi.New(t)
	fake.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{})
	c, err := api.NewClient(api.ClientOptions{
		BaseURL: fake.URL(),
		TokenFunc: func(_ context.Context, _ string) (string, string, error) {
			return "", "", fmt.Errorf("simulated lookup failure")
		},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = c.REST(context.Background(), "GET", "user", nil, nil)
	if err == nil {
		t.Fatal("expected TokenFunc error to propagate")
	}
	if !strings.Contains(err.Error(), "simulated") {
		t.Errorf("error should chain TokenFunc's: %v", err)
	}
}

// Context cancellation ---------------------------------------------------

func TestContextCancelInterruptsRetry(t *testing.T) {
	fake := fakeapi.New(t)
	fake.Handle("GET", "/api/v1/slow", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(502)
	})

	c, _ := api.NewClient(api.ClientOptions{
		BaseURL:    fake.URL(),
		TokenFunc:  func(_ context.Context, _ string) (string, string, error) { return "t", "test", nil },
		MaxRetries: api.IntPtr(5),
		Timeout:    2 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before issuing

	if err := c.REST(ctx, "GET", "slow", nil, nil); err == nil {
		t.Fatal("expected cancellation error")
	}
}
