// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fakeapi wraps httptest.Server with helpers tailored to
// shithub-cli's command-level tests. Every later sprint's tests build a
// Server via New, register JSON responses with RegisterJSON, and pass the
// Server.Client() to NewClient. The handlers record every call so tests
// can assert which endpoints fired and with what bodies.
package fakeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Server is a one-stop fake shithub HTTP server. Construct via New;
// register routes with RegisterJSON or Handle; tear down via t.Cleanup
// (registered automatically by New).
type Server struct {
	t  *testing.T
	hs *httptest.Server
	mu sync.Mutex

	// routes maps "METHOD PATH" (e.g. "GET /api/v1/user") to a handler.
	// PATH may include ":x" placeholders interpreted as wildcards.
	routes map[string]http.HandlerFunc

	// calls records every request received so tests can assert on volume,
	// order, and body content.
	calls []Call
}

// Call captures one inbound request for assertion.
type Call struct {
	Method string
	Path   string
	Query  string
	Body   []byte
	Header http.Header
}

// New creates a server, registers t.Cleanup to close it, and returns it
// ready to accept route registrations. The dev-insecure-HTTP env var is
// flipped on under t.Setenv (auto-restored on test end) so api.NewClient
// will accept the httptest server's http:// URL without ceremony.
func New(t *testing.T) *Server {
	t.Helper()
	t.Setenv(api.EnvInsecureHTTP, "1")
	s := &Server{
		t:      t,
		routes: map[string]http.HandlerFunc{},
	}
	s.hs = httptest.NewServer(http.HandlerFunc(s.dispatch))
	t.Cleanup(s.hs.Close)
	return s
}

// URL returns the server's base URL. Pass this to ClientOptions.BaseURL
// when constructing an api.Client against the fake.
func (s *Server) URL() string { return s.hs.URL }

// NewClient builds an api.Client configured to talk to the fake. The
// returned client's TokenFunc yields a fixed test token; replace via
// NewClientWithToken when a test needs a different value.
func (s *Server) NewClient() *api.Client {
	return s.NewClientWithToken("shithub_pat_testtoken")
}

// NewClientWithToken builds a client whose TokenFunc returns the given
// token literal. Useful for verifying header redaction and for tests
// that need to assert "this request used the expected token".
func (s *Server) NewClientWithToken(token string) *api.Client {
	c, err := api.NewClient(api.ClientOptions{
		BaseURL: s.URL(),
		Host:    "fake.shithub.local",
		TokenFunc: func(ctx context.Context, host string) (string, string, error) {
			return token, "test", nil
		},
		MaxRetries: api.IntPtr(1), // keep tests snappy; raise per-test if needed
		Timeout:    5_000_000_000, // 5s
		// HTTPClient defaults to one with the right timeout; the httptest
		// server is reachable via the default transport.
	})
	if err != nil {
		s.t.Fatalf("fakeapi: NewClient: %v", err)
	}
	return c
}

// RegisterJSON registers a fixed-response handler for the given method+path.
// Body is JSON-marshaled; headers passed via extra are merged onto the response.
//
// Path matching is exact (no placeholder interpolation); subsequent calls
// to the same key replace the registration.
func (s *Server) RegisterJSON(method, path string, status int, body any, extraHeaders ...map[string]string) {
	s.t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		s.t.Fatalf("fakeapi: marshal %s %s: %v", method, path, err)
	}
	s.Handle(method, path, func(w http.ResponseWriter, r *http.Request) {
		for _, h := range extraHeaders {
			for k, v := range h {
				w.Header().Set(k, v)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(encoded)
	})
}

// Handle registers a custom handler for method+path. The handler is
// invoked from within dispatch under no lock; it MUST be safe to call
// concurrently if the test issues parallel requests.
func (s *Server) Handle(method, path string, handler http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[method+" "+path] = handler
}

// dispatch is the muxer Server registers with httptest. Records the call,
// looks up the route, and writes 404 with a shithub-shaped envelope on miss.
func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	s.mu.Lock()
	s.calls = append(s.calls, Call{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   body,
		Header: r.Header.Clone(),
	})
	handler, ok := s.routes[r.Method+" "+r.URL.Path]
	s.mu.Unlock()

	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"fakeapi: unregistered route"}`))
		return
	}

	// Reinstall the body for the handler, since dispatch consumed it.
	r.Body = io.NopCloser(bytes.NewReader(body))
	handler(w, r)
}

// Calls returns a snapshot of every recorded inbound request. The slice
// is safe to read without further locking; tests typically iterate and
// assert on individual entries.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Call, len(s.calls))
	copy(out, s.calls)
	return out
}

// AssertCalled fails the test if no recorded call matches method+path.
func (s *Server) AssertCalled(method, path string) {
	s.t.Helper()
	for _, c := range s.Calls() {
		if c.Method == method && c.Path == path {
			return
		}
	}
	s.t.Errorf("fakeapi: expected call %s %s; calls=%v", method, path, s.Calls())
}

// AssertCallCount fails the test when the recorded volume does not match.
func (s *Server) AssertCallCount(want int) {
	s.t.Helper()
	if got := len(s.Calls()); got != want {
		s.t.Errorf("fakeapi: call count: want %d got %d (%v)", want, got, s.Calls())
	}
}
