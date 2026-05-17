// SPDX-License-Identifier: AGPL-3.0-or-later

package status

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

func newOpts(t *testing.T) (*Options, *cmdutiltest.Factory) {
	t.Helper()
	tf := cmdutiltest.New(t)
	return &Options{
		IO:         tf.IOStreams,
		Hosts:      tf.Factory.Hosts,
		Keyring:    tf.Factory.Keyring,
		HTTPClient: tf.Factory.HTTPClient,
	}, tf
}

// TestStatusEmptyHostsExitsNonZero is the C2 regression: previously
// `auth status` with no hosts configured exited 0 with a stdout/stderr
// message — CI gating on `shithub auth status >/dev/null` silently
// succeeded. Now it errors so the exit code matches the message.
func TestStatusEmptyHostsExitsNonZero(t *testing.T) {
	opts, _ := newOpts(t)
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when no hosts configured")
	}
	if !strings.Contains(err.Error(), "no hosts configured") {
		t.Errorf("error should mention no hosts configured; got: %v", err)
	}
}

// TestStatusUnknownHostnameExitsNonZero is the C3 regression: passing
// --hostname for a host that isn't in hosts.yml said "no hosts
// configured" (misleading: there ARE hosts, just not that one) and
// exited 0. Now it errors with a hostname-specific message.
func TestStatusUnknownHostnameExitsNonZero(t *testing.T) {
	opts, tf := newOpts(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	_ = tf.Config.Hosts.Save()
	opts.Hostname = "example.com"

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when --hostname is not configured")
	}
	if !strings.Contains(err.Error(), "not logged into example.com") {
		t.Errorf("error should name the missing host; got: %v", err)
	}
}

func TestStatusHumanRendering(t *testing.T) {
	opts, tf := newOpts(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	tf.Config.Hosts.Get("shithub.sh").Default = true
	_ = config.SetToken(tf.Config.Keyring, "shithub.sh", "mf", "shithub_pat_xyz")
	_ = tf.Config.Hosts.Save()

	tf.Server.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo:read, user:read")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1,"username":"mf"}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	want := []string{
		"shithub.sh",
		"logged in as mf",
		"Token source: keyring",
		"Scopes:       repo:read, user:read",
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in output:\n%s", w, out)
		}
	}
}

func TestStatusJSONShape(t *testing.T) {
	opts, tf := newOpts(t)
	opts.JSON = true
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	tf.Config.Hosts.Get("shithub.sh").Default = true
	_ = config.SetToken(tf.Config.Keyring, "shithub.sh", "mf", "shithub_pat_xyz")
	_ = tf.Config.Hosts.Save()
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	for _, key := range []string{`"host":`, `"user":`, `"token_source":`, `"active":`, `"reachable":`} {
		if !strings.Contains(out, key) {
			t.Errorf("JSON output missing field %s:\n%s", key, out)
		}
	}
}

func TestStatusShowTokenOnTTYRefuses(t *testing.T) {
	opts, tf := newOpts(t)
	opts.ShowToken = true
	tf.IOStreams.SetStdoutTTY(true)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	_ = tf.Config.Hosts.Save()

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected refusal on TTY")
	}
	if !strings.Contains(err.Error(), "leak the token") {
		t.Errorf("error should explain refusal, got: %v", err)
	}
}

func TestStatusActiveFlagExitCode(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Active = true
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	tf.Config.Hosts.Get("shithub.sh").Default = true
	_ = config.SetToken(tf.Config.Keyring, "shithub.sh", "mf", "shithub_pat_xyz")
	_ = tf.Config.Hosts.Save()
	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{"id": 1, "username": "mf"})

	if err := Run(context.Background(), opts); err != nil {
		t.Errorf("--active with valid host should exit 0, got: %v", err)
	}
}
