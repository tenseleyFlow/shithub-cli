// SPDX-License-Identifier: AGPL-3.0-or-later

package add

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

func TestGPGAddArmoredStdin(t *testing.T) {
	tf := cmdutiltest.New(t)
	const blob = "-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmQENBF…\n-----END PGP PUBLIC KEY BLOCK-----\n"
	tf.In.WriteString(blob)

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/user/gpg_keys", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keys.GPGKey{ID: 5, KeyID: "AAAA1111"})
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(string(body), "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Errorf("armored block not sent: %s", body)
	}
}

// TestGPGAddRefusesPrivate is the safety check.
func TestGPGAddRefusesPrivate(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.In.WriteString("-----BEGIN PGP PRIVATE KEY BLOCK-----\n\nlQOYBF…\n-----END PGP PRIVATE KEY BLOCK-----\n")

	tf.Server.Handle(http.MethodPost, "/api/v1/user/gpg_keys", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("private GPG key should never reach the server")
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("want private-key refusal; got %v", err)
	}
}

func TestGPGAddRefusesNonArmored(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.In.WriteString("ssh-ed25519 AAAA mykey\n")

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error: SSH key shouldn't pass as GPG armored block")
	}
}

// TestGPGAddTitleAliasMapsToName pins F31: cobra accepts both `--name`
// and `--title` and writes to the same opts.Name target so gh-style
// `--title` works across both key-management commands.
func TestGPGAddTitleAliasMapsToName(t *testing.T) {
	tf := cmdutiltest.New(t)
	const blob = "-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmQENBF…\n-----END PGP PUBLIC KEY BLOCK-----\n"
	tf.In.WriteString(blob)

	var body json.RawMessage
	tf.Server.Handle(http.MethodPost, "/api/v1/user/gpg_keys", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keys.GPGKey{ID: 7})
	})

	cmd := NewCmd(tf.Factory)
	cmd.SetArgs([]string{"--title", "release-key"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(string(body), `"name":"release-key"`) {
		t.Errorf("body should send name=release-key via --title alias: %s", body)
	}
}
