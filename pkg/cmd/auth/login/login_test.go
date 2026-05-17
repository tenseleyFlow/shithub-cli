// SPDX-License-Identifier: AGPL-3.0-or-later

package login

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/auth/device"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// newOpts wires the Options to the test factory's collaborators.
func newOpts(t *testing.T) (*Options, *cmdutiltest.Factory) {
	t.Helper()
	tf := cmdutiltest.New(t)
	return &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
		NewCandidateClient: func(_, token string) (*api.Client, error) {
			return tf.Server.NewClientWithToken(token), nil
		},
	}, tf
}

func TestLoginWithTokenHappyPath(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	opts.InsecureStorage = true // skip keyring path; in-memory fake works regardless

	tf.In.WriteString("shithub_pat_abc\n")

	tf.Server.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token shithub_pat_abc" {
			t.Errorf("Authorization: got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("X-OAuth-Scopes", "repo:read, repo:write")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1,"username":"mf"}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Hosts.yml persisted the entry under the requested host.
	hosts, _ := tf.Factory.Hosts()
	entry, ok := hosts["shithub.sh"]
	if !ok {
		t.Fatal("hosts.yml did not gain a shithub.sh entry")
	}
	if entry.User != "mf" {
		t.Errorf("User: got %q", entry.User)
	}
	if !entry.InsecureStorage || entry.OAuthToken != "shithub_pat_abc" {
		t.Errorf("expected token persisted to hosts.yml insecure path; got %+v", entry)
	}
	if !entry.Default {
		t.Error("first login should set Default=true")
	}
	if !strings.Contains(tf.ErrOut.String(), "Authenticated to shithub.sh as mf") {
		t.Errorf("ErrOut missing success line: %q", tf.ErrOut.String())
	}
}

// TestLoginWarnsOnGitHubTokenShape covers the audit #136 hint: pasting
// a `gh_pat_…` / `ghp_…` token surfaces a stderr warning *before* the
// server roundtrip, sparing the user a confusing 401.
func TestLoginWarnsOnGitHubTokenShape(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	opts.InsecureStorage = true
	tf.In.WriteString("ghp_abcdef\n")
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	_ = Run(context.Background(), opts)
	if !strings.Contains(tf.ErrOut.String(), "looks like a GitHub token") {
		t.Errorf("expected GitHub-shape warning; got %q", tf.ErrOut.String())
	}
}

// TestLoginWarnsOnUnknownPrefix: any token without the shithub_pat_
// prefix gets a soft warning. The flow continues so OAuth tokens (C04a)
// still work when the server starts emitting them.
func TestLoginWarnsOnUnknownPrefix(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	opts.InsecureStorage = true
	tf.In.WriteString("random_xyz\n")
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	_ = Run(context.Background(), opts)
	if !strings.Contains(tf.ErrOut.String(), "doesn't start with shithub_pat_") {
		t.Errorf("expected unknown-prefix warning; got %q", tf.ErrOut.String())
	}
}

// TestLoginSuppressesWarningOnShithubPAT: the canonical shape doesn't
// produce the warning, so `auth login` stays quiet in the happy path.
func TestLoginSuppressesWarningOnShithubPAT(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	opts.InsecureStorage = true
	tf.In.WriteString("shithub_pat_abcdef\n")
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(tf.ErrOut.String(), "doesn't start with") ||
		strings.Contains(tf.ErrOut.String(), "looks like a GitHub token") {
		t.Errorf("unexpected token-shape warning on canonical PAT: %q", tf.ErrOut.String())
	}
}

func TestLoginWithTokenStoresInKeyring(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true

	tf.In.WriteString("shithub_pat_xyz\n")

	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !tf.Config.Keyring.Has("shithub:shithub.sh", "mf") {
		t.Error("keyring should have token after non-insecure login")
	}
	hosts, _ := tf.Factory.Hosts()
	if entry := hosts["shithub.sh"]; entry.OAuthToken != "" || entry.InsecureStorage {
		t.Errorf("OAuthToken should be empty when keyring used; got %+v", entry)
	}
}

func TestLoginRejects401(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	tf.In.WriteString("shithub_pat_bogus\n")

	tf.Server.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	})

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !strings.Contains(err.Error(), "token rejected") {
		t.Errorf("error should mention token rejection, got: %v", err)
	}
	// Nothing should have been persisted.
	hosts, _ := tf.Factory.Hosts()
	if _, ok := hosts["shithub.sh"]; ok {
		t.Error("hosts.yml should not gain an entry on failed login")
	}
}

func TestLoginRefusesGithubCom(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Hostname = "github.com"
	opts.WithToken = true

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected refusal for github.com")
	}
	if !strings.Contains(err.Error(), "not a shithub host") {
		t.Errorf("error should explain refusal, got: %v", err)
	}
}

func TestLoginRefusesBadGitProtocol(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.GitProtocol = "ftp"
	opts.WithToken = true

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected refusal for bad protocol")
	}
}

func TestLoginEmptyStdinTokenFails(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	// In remains empty.

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on empty stdin")
	}
}

func TestLoginInteractivePromptInNonInteractiveErrors(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Hostname = "shithub.sh"
	// No --with-token; Test() streams report NeverPrompt() so the prompt
	// path refuses before touching survey.
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error in non-interactive context")
	}
}

func TestLoginHonorsGitProtocol(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	opts.GitProtocol = config.GitProtocolSSH
	tf.In.WriteString("shithub_pat_abc\n")
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	hosts, _ := tf.Factory.Hosts()
	if hosts["shithub.sh"].GitProtocol != "ssh" {
		t.Errorf("GitProtocol: got %q", hosts["shithub.sh"].GitProtocol)
	}
}

// setupDeviceFlowServer stands up an httptest.Server that serves the two
// RFC 8628 endpoints. issueDelay controls how many `authorization_pending`
// rounds the exchange endpoint replies with before returning the token.
func setupDeviceFlowServer(t *testing.T, token string, pendingRounds int32) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(device.CodeResponse{
			DeviceCode:              "dc-test",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "http://" + r.Host + "/login/device",
			VerificationURIComplete: "http://" + r.Host + "/login/device?user_code=ABCD-EFGH",
			ExpiresIn:               900,
			Interval:                0,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n <= pendingRounds {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(device.TokenResponse{
			AccessToken: token,
			TokenType:   "bearer",
			Scope:       "user:read,repo:read",
		})
	})
	return httptest.NewServer(mux)
}

// newDeviceOpts mirrors newOpts but also wires the device-flow seam at
// srv and flips NeverPrompt so the Confirm before browser-open fires.
func newDeviceOpts(t *testing.T, srv *httptest.Server) (*Options, *cmdutiltest.Factory) {
	t.Helper()
	opts, tf := newOpts(t)
	tf.IOStreams.SetNeverPrompt(false)
	opts.NewDeviceClient = func(_ string) (*device.Client, error) {
		t.Setenv(device.EnvInsecureHTTP, "1")
		return device.NewClient(device.Options{
			BaseURL: srv.URL,
			Sleep:   func(_ context.Context, _ time.Duration) error { return nil },
		})
	}
	// Default: skip browser-open in tests; tests that exercise the
	// opener inject their own closure.
	opts.OpenBrowser = func(_ string) error { return nil }
	return opts, tf
}

func TestLoginDeviceFlowHappyPath(t *testing.T) {
	srv := setupDeviceFlowServer(t, "shithub_pat_dev", 1)
	defer srv.Close()
	opts, tf := newDeviceOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true
	tf.Prompt.QueueConfirm(true) // approve the "open browser" prompt
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	hosts, _ := tf.Factory.Hosts()
	entry := hosts["shithub.sh"]
	if entry == nil || entry.OAuthToken != "shithub_pat_dev" {
		t.Errorf("device-flow token not persisted; entry=%+v", entry)
	}
	if !strings.Contains(tf.ErrOut.String(), "ABCD-EFGH") {
		t.Errorf("ErrOut missing user_code: %q", tf.ErrOut.String())
	}
	if !strings.Contains(tf.ErrOut.String(), "Authenticated to shithub.sh as mf") {
		t.Errorf("ErrOut missing success line: %q", tf.ErrOut.String())
	}
}

func TestLoginDeviceFlowOpensBrowserOnConsent(t *testing.T) {
	srv := setupDeviceFlowServer(t, "shithub_pat_x", 0)
	defer srv.Close()
	opts, tf := newDeviceOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true
	tf.Prompt.QueueConfirm(true)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	var openedURL string
	opts.OpenBrowser = func(u string) error { openedURL = u; return nil }

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(openedURL, "/login/device?user_code=ABCD-EFGH") {
		t.Errorf("OpenBrowser called with %q, want the verification_uri_complete", openedURL)
	}
}

func TestLoginDeviceFlowSkipsBrowserOnDecline(t *testing.T) {
	srv := setupDeviceFlowServer(t, "shithub_pat_x", 0)
	defer srv.Close()
	opts, tf := newDeviceOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true
	tf.Prompt.QueueConfirm(false)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	called := false
	opts.OpenBrowser = func(_ string) error { called = true; return nil }

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called {
		t.Error("OpenBrowser should not be called when user declines")
	}
	if !strings.Contains(tf.ErrOut.String(), "Open the URL above") {
		t.Errorf("ErrOut missing manual-open hint: %q", tf.ErrOut.String())
	}
}

func TestLoginDeviceFlowForwardsScopes(t *testing.T) {
	var gotScope string
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotScope = r.PostForm.Get("scope")
		_ = json.NewEncoder(w).Encode(device.CodeResponse{
			DeviceCode:      "dc",
			UserCode:        "AAAA-BBBB",
			VerificationURI: "http://" + r.Host + "/login/device",
			ExpiresIn:       900, Interval: 0,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(device.TokenResponse{
			AccessToken: "shithub_pat_s",
			TokenType:   "bearer",
			Scope:       "repo:write",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	opts, tf := newDeviceOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true
	opts.Scopes = "repo:read repo:write"
	tf.Prompt.QueueConfirm(false)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotScope != "repo:read repo:write" {
		t.Errorf("server received scope=%q, want %q", gotScope, "repo:read repo:write")
	}
}

func TestLoginDeviceFlowAccessDeniedSurfacesHint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(device.CodeResponse{
			DeviceCode: "dc", UserCode: "AAAA-BBBB",
			VerificationURI: "http://" + r.Host + "/login/device",
			ExpiresIn:       900, Interval: 0,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	opts, tf := newDeviceOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true
	tf.Prompt.QueueConfirm(false)

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on access_denied")
	}
	if !strings.Contains(err.Error(), "denied in the browser") {
		t.Errorf("error message lost the access_denied hint: %v", err)
	}
	// No token persisted on failure.
	hosts, _ := tf.Factory.Hosts()
	if _, ok := hosts["shithub.sh"]; ok {
		t.Error("hosts.yml should not gain an entry on access_denied")
	}
}

func TestLoginWithTokenAndScopesRejected(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	opts.Scopes = "repo:read"
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "device-flow option") {
		t.Errorf("err = %v, want '--scopes is a device-flow option…'", err)
	}
}

func TestLoginWithTokenAndWebMutuallyExclusive(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Hostname = "shithub.sh"
	opts.WithToken = true
	opts.Web = true
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("err = %v, want 'mutually exclusive'", err)
	}
}
