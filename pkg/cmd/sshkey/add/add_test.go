// SPDX-License-Identifier: AGPL-3.0-or-later

package add

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

func TestAddReadsFile(t *testing.T) {
	tf := cmdutiltest.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "id_ed25519.pub")
	const blob = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample mf@laptop\n"
	if err := os.WriteFile(path, []byte(blob), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/user/keys", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keys.SSHKey{ID: 1, Title: "mf@laptop", Fingerprint: "SHA256:abc"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Path:        path,
		Type:        keys.SSHKindAuthentication,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), `"kind":"authentication"`) {
		t.Errorf("kind not sent: %s", body)
	}
	// Comment derived from the key's trailing token.
	if !strings.Contains(string(body), `"title":"mf@laptop"`) {
		t.Errorf("default title not derived from comment: %s", body)
	}
}

func TestAddReadsStdin(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.In.WriteString("ssh-ed25519 AAAA mykey\n")

	tf.Server.Handle(http.MethodPost, "/api/v1/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keys.SSHKey{ID: 2, Title: "mykey"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Path:        "-",
		Title:       "explicit",
		Type:        keys.SSHKindAuthentication,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestAddRefusesPrivateKey is the safety check: pasting a private key
// must not reach the wire.
func TestAddRefusesPrivateKey(t *testing.T) {
	tf := cmdutiltest.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "id_ed25519")
	priv := "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk…\n-----END OPENSSH PRIVATE KEY-----\n"
	if err := os.WriteFile(path, []byte(priv), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tf.Server.Handle(http.MethodPost, "/api/v1/user/keys", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("private-key upload should never reach the server")
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Path:        path,
		Type:        keys.SSHKindAuthentication,
	}
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("want private-key refusal; got %v", err)
	}
}

func TestAddRejectsBadType(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Path:        "-",
		Type:        "bogus",
	}
	tf.In.WriteString("ssh-ed25519 AAAA mykey\n")
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error for bad --type")
	}
}
