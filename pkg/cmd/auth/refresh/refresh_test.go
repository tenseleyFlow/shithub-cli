// SPDX-License-Identifier: AGPL-3.0-or-later

package refresh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/auth/device"
	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// newOpts wires Options against the test factory + a device-flow
// httptest server. Tests that don't need device flow can ignore srv.
func newOpts(t *testing.T, srv *httptest.Server) (*Options, *cmdutiltest.Factory) {
	t.Helper()
	tf := cmdutiltest.New(t)
	tf.IOStreams.SetNeverPrompt(false)
	// CI=true on GitHub Actions would trip the CI-refusal guard;
	// dedicated tests that exercise it re-Setenv after.
	t.Setenv("CI", "")
	o := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
		NewCandidateClient: func(_, token string) (*api.Client, error) {
			return tf.Server.NewClientWithToken(token), nil
		},
		OpenBrowser: func(_ string) error { return nil },
	}
	if srv != nil {
		o.NewDeviceClient = func(_ string) (*device.Client, error) {
			t.Setenv(device.EnvInsecureHTTP, "1")
			return device.NewClient(device.Options{
				BaseURL: srv.URL,
				Sleep:   func(_ context.Context, _ time.Duration) error { return nil },
			})
		}
	}
	return o, tf
}

// stubDeviceFlow stands up a happy-path device-flow server. token is
// the access_token the access_token endpoint will mint after one
// authorization_pending round; user is the login the validate call
// will report.
func stubDeviceFlow(t *testing.T, token string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	pending := true
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(device.CodeResponse{
			DeviceCode:              "dc",
			UserCode:                "AAAA-BBBB",
			VerificationURI:         "http://" + r.Host + "/login/device",
			VerificationURIComplete: "http://" + r.Host + "/login/device?user_code=AAAA-BBBB",
			ExpiresIn:               900,
			Interval:                0,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		if pending {
			pending = false
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(device.TokenResponse{
			AccessToken: token,
			TokenType:   "bearer",
			Scope:       "user:read,repo:read",
		})
	})
	return httptest.NewServer(mux)
}

func TestRefreshNoHostConfigured(t *testing.T) {
	opts, _ := newOpts(t, nil)
	opts.NewDeviceClient = func(_ string) (*device.Client, error) { return nil, nil }
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "no hosts configured") {
		t.Errorf("err = %v, want 'no hosts configured'", err)
	}
}

func TestRefreshNoExistingLogin(t *testing.T) {
	srv := stubDeviceFlow(t, "shithub_pat_new")
	defer srv.Close()
	opts, tf := newOpts(t, srv)
	// Seed a host but with empty entry (no login).
	hosts, _ := tf.Factory.Hosts()
	hosts["shithub.sh"] = &config.HostEntry{}
	opts.Hostname = "shithub.sh"
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "no existing login") {
		t.Errorf("err = %v, want 'no existing login'", err)
	}
}

func TestRefreshRotatesTokenAndPreservesUser(t *testing.T) {
	srv := stubDeviceFlow(t, "shithub_pat_rotated")
	defer srv.Close()
	opts, tf := newOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true

	// Pre-seed a logged-in host.
	hosts, _ := tf.Factory.Hosts()
	hosts["shithub.sh"] = &config.HostEntry{
		User:            "mf",
		OAuthToken:      "shithub_pat_old",
		InsecureStorage: true,
		LastScopes:      []string{"user:read"},
		Default:         true,
	}
	tf.Prompt.QueueConfirm(false) // skip browser open
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := hosts["shithub.sh"]
	if got.OAuthToken != "shithub_pat_rotated" {
		t.Errorf("token not rotated: %+v", got)
	}
	if got.User != "mf" {
		t.Errorf("User: %q, want unchanged 'mf'", got.User)
	}
	if !strings.Contains(tf.ErrOut.String(), "Refreshed credentials for shithub.sh as mf") {
		t.Errorf("ErrOut missing success line: %q", tf.ErrOut.String())
	}
}

func TestRefreshUsesActiveHostWhenSingle(t *testing.T) {
	srv := stubDeviceFlow(t, "shithub_pat_x")
	defer srv.Close()
	opts, tf := newOpts(t, srv)
	hosts, _ := tf.Factory.Hosts()
	hosts["shithub.sh"] = &config.HostEntry{
		User:            "mf",
		OAuthToken:      "shithub_pat_old",
		InsecureStorage: true,
	}
	tf.Prompt.QueueConfirm(false)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hosts["shithub.sh"].OAuthToken != "shithub_pat_x" {
		t.Errorf("token not rotated: %+v", hosts["shithub.sh"])
	}
}

func TestRefreshRefusesWhenServerSwapsUser(t *testing.T) {
	srv := stubDeviceFlow(t, "shithub_pat_x")
	defer srv.Close()
	opts, tf := newOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true
	hosts, _ := tf.Factory.Hosts()
	hosts["shithub.sh"] = &config.HostEntry{
		User:            "alice",
		OAuthToken:      "shithub_pat_alice",
		InsecureStorage: true,
	}
	tf.Prompt.QueueConfirm(false)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 2, "username": "bob"})

	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "logged in as") {
		t.Errorf("err = %v, want refusal on user mismatch", err)
	}
	// Token should NOT have been rotated to alice's slot.
	if hosts["shithub.sh"].OAuthToken != "shithub_pat_alice" {
		t.Errorf("token unexpectedly rotated despite user mismatch: %+v", hosts["shithub.sh"])
	}
}

func TestRefreshForwardsExplicitScopes(t *testing.T) {
	var gotScope string
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotScope = r.PostForm.Get("scope")
		_ = json.NewEncoder(w).Encode(device.CodeResponse{
			DeviceCode: "dc", UserCode: "AAAA-BBBB",
			VerificationURI: "http://" + r.Host + "/login/device",
			ExpiresIn:       900, Interval: 0,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(device.TokenResponse{
			AccessToken: "shithub_pat_new",
			TokenType:   "bearer",
			Scope:       "repo:write",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	opts, tf := newOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.Scopes = "repo:write user:read"
	opts.InsecureStorage = true
	hosts, _ := tf.Factory.Hosts()
	hosts["shithub.sh"] = &config.HostEntry{
		User: "mf", OAuthToken: "old", InsecureStorage: true,
		LastScopes: []string{"user:read"},
	}
	tf.Prompt.QueueConfirm(false)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotScope != "repo:write user:read" {
		t.Errorf("scope sent: %q, want %q", gotScope, "repo:write user:read")
	}
}

func TestRefreshDefaultsScopesFromLastScopes(t *testing.T) {
	var gotScope string
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotScope = r.PostForm.Get("scope")
		_ = json.NewEncoder(w).Encode(device.CodeResponse{
			DeviceCode: "dc", UserCode: "AAAA-BBBB",
			VerificationURI: "http://" + r.Host + "/login/device",
			ExpiresIn:       900, Interval: 0,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(device.TokenResponse{
			AccessToken: "shithub_pat_n",
			TokenType:   "bearer",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	opts, tf := newOpts(t, srv)
	opts.Hostname = "shithub.sh"
	opts.InsecureStorage = true
	hosts, _ := tf.Factory.Hosts()
	hosts["shithub.sh"] = &config.HostEntry{
		User: "mf", OAuthToken: "old", InsecureStorage: true,
		LastScopes: []string{"repo:read", "user:read"},
	}
	tf.Prompt.QueueConfirm(false)
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotScope != "repo:read user:read" {
		t.Errorf("scope sent: %q, want %q", gotScope, "repo:read user:read")
	}
}

func TestRefreshAmbiguousHostErrors(t *testing.T) {
	opts, tf := newOpts(t, nil)
	opts.NewDeviceClient = func(_ string) (*device.Client, error) { return nil, nil }
	hosts, _ := tf.Factory.Hosts()
	hosts["a.example"] = &config.HostEntry{User: "x"}
	hosts["b.example"] = &config.HostEntry{User: "y"}
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "multiple hosts configured") {
		t.Errorf("err = %v, want 'multiple hosts configured'", err)
	}
}

func TestRefreshRefusesUnderCI(t *testing.T) {
	srv := stubDeviceFlow(t, "shithub_pat_x")
	defer srv.Close()
	opts, tf := newOpts(t, srv)
	opts.Hostname = "shithub.sh"
	hosts, _ := tf.Factory.Hosts()
	hosts["shithub.sh"] = &config.HostEntry{User: "mf", OAuthToken: "old", InsecureStorage: true}
	// Set CI *after* newOpts (which clears it).
	t.Setenv("CI", "1")

	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "CI=1") {
		t.Errorf("err = %v, want CI refusal", err)
	}
}
