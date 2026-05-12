// SPDX-License-Identifier: AGPL-3.0-or-later

package set

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/nacl/box"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
)

// TestSetRepoSecretSealedBox verifies the encrypt-and-PUT happy path:
// public-key fetched, ciphertext non-empty, key_id echoed, plaintext
// field absent.
func TestSetRepoSecretSealedBox(t *testing.T) {
	tf := cmdutiltest.New(t)
	pub, _, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub[:])

	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/actions/secrets/public-key", 200,
		secrets.PublicKey{KeyID: "k-1", Key: pubB64})
	var putBody secrets.SetSecretInput
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/actions/secrets/FOO", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &putBody)
		w.WriteHeader(http.StatusNoContent)
	})

	tf.In.WriteString("hunter2")
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "FOO",
		Repo:        "o/r",
		App:         "actions",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if putBody.EncryptedValue == "" {
		t.Errorf("EncryptedValue empty: %+v", putBody)
	}
	if putBody.KeyID != "k-1" {
		t.Errorf("KeyID: want k-1 got %q", putBody.KeyID)
	}
	if putBody.PlaintextValue != "" {
		t.Errorf("PlaintextValue should be empty in sealed-box mode: %q", putBody.PlaintextValue)
	}
}

// TestSetRepoSecretPlaintextFallback ensures a 404 on the public-key
// endpoint falls back to a plaintext PUT and surfaces the warning.
func TestSetRepoSecretPlaintextFallback(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.Handle(http.MethodGet, "/api/v1/repos/o/r/actions/secrets/public-key", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	var putBody secrets.SetSecretInput
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/actions/secrets/FOO", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &putBody)
		w.WriteHeader(http.StatusNoContent)
	})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "FOO",
		Repo:        "o/r",
		App:         "actions",
		Body:        "hunter2",
		BodySet:     true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if putBody.PlaintextValue != "hunter2" {
		t.Errorf("PlaintextValue: want hunter2 got %q", putBody.PlaintextValue)
	}
	if putBody.EncryptedValue != "" {
		t.Errorf("EncryptedValue should be empty in plaintext mode: %q", putBody.EncryptedValue)
	}
	if !strings.Contains(tf.ErrOut.String(), "plaintext") {
		t.Errorf("warning missing from stderr: %q", tf.ErrOut.String())
	}
}

// TestSetRejectsUnsupportedApp keeps the --app flag locked to actions.
func TestSetRejectsUnsupportedApp(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		Name:        "FOO",
		Repo:        "o/r",
		App:         "dependabot",
		Body:        "v",
		BodySet:     true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("Run: expected error for --app=dependabot")
	}
}

// TestReadValueSources isolates the source-precedence ordering.
func TestReadValueSources(t *testing.T) {
	tf := cmdutiltest.New(t)
	// --body wins
	opts := &options{IO: tf.IOStreams, Body: "from-body", BodySet: true}
	got, err := readValue(opts)
	if err != nil || string(got) != "from-body" {
		t.Errorf("--body: got %q err=%v", got, err)
	}
	// Empty --body still honored
	opts = &options{IO: tf.IOStreams, BodySet: true}
	got, err = readValue(opts)
	if err != nil || string(got) != "" {
		t.Errorf("empty --body: got %q err=%v", got, err)
	}
	// stdin
	tf2 := cmdutiltest.New(t)
	tf2.In.WriteString("from-stdin")
	opts = &options{IO: tf2.IOStreams}
	got, err = readValue(opts)
	if err != nil || string(got) != "from-stdin" {
		t.Errorf("stdin: got %q err=%v", got, err)
	}
}
