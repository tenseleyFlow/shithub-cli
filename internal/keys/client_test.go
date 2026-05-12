// SPDX-License-Identifier: AGPL-3.0-or-later

package keys

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
)

func TestListSSHDecodes(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/user/keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]SSHKey{
			{ID: 1, Title: "laptop", Kind: SSHKindAuthentication, Fingerprint: "SHA256:abc"},
			{ID: 2, Title: "yubikey", Kind: SSHKindSigning},
		})
	})

	c := NewClient(srv.NewClient())
	out, err := c.ListSSH(context.Background())
	if err != nil {
		t.Fatalf("ListSSH: %v", err)
	}
	if len(out) != 2 || out[0].Title != "laptop" || out[1].Kind != SSHKindSigning {
		t.Errorf("got %+v", out)
	}
}

func TestAddSSHPostsBody(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/user/keys", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(SSHKey{ID: 7, Title: "laptop"})
	})

	c := NewClient(srv.NewClient())
	out, err := c.AddSSH(context.Background(), SSHKeyInput{
		Title: "laptop", Key: "ssh-ed25519 AAAA", Kind: SSHKindAuthentication,
	})
	if err != nil {
		t.Fatalf("AddSSH: %v", err)
	}
	if out.ID != 7 {
		t.Errorf("server response: %+v", out)
	}
	if !strings.Contains(string(body), `"title":"laptop"`) || !strings.Contains(string(body), `"kind":"authentication"`) {
		t.Errorf("body: %s", body)
	}
}

func TestDeleteSSHHitsEndpoint(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodDelete, "/api/v1/user/keys/42", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c := NewClient(srv.NewClient())
	if err := c.DeleteSSH(context.Background(), 42); err != nil {
		t.Fatalf("DeleteSSH: %v", err)
	}
	srv.AssertCalled(http.MethodDelete, "/api/v1/user/keys/42")
}

func TestListGPGDecodes(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/user/gpg_keys", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]GPGKey{
			{ID: 1, KeyID: "AAAA1111", CanSign: true, CanEncryptComms: true, Emails: []GPGEmail{{Email: "mf@x", Verified: true}}},
		})
	})
	c := NewClient(srv.NewClient())
	out, err := c.ListGPG(context.Background())
	if err != nil {
		t.Fatalf("ListGPG: %v", err)
	}
	if len(out) != 1 || out[0].KeyID != "AAAA1111" || !out[0].CanSign {
		t.Errorf("got %+v", out)
	}
}

func TestAddGPGPostsBody(t *testing.T) {
	srv := fakeapi.New(t)
	var body json.RawMessage
	srv.Handle(http.MethodPost, "/api/v1/user/gpg_keys", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GPGKey{ID: 5, KeyID: "AAAA"})
	})
	c := NewClient(srv.NewClient())
	out, err := c.AddGPG(context.Background(), GPGKeyInput{
		ArmoredPublicKey: "-----BEGIN PGP PUBLIC KEY BLOCK-----\nmQENBF…\n-----END PGP PUBLIC KEY BLOCK-----",
	})
	if err != nil {
		t.Fatalf("AddGPG: %v", err)
	}
	if out.ID != 5 {
		t.Errorf("got %+v", out)
	}
	if !strings.Contains(string(body), "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Errorf("body: %s", body)
	}
}
