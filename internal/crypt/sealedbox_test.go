// SPDX-License-Identifier: AGPL-3.0-or-later

package crypt

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

// TestSealAnonymousRoundTrip generates a curve25519 keypair, seals
// against the public half, and decrypts with the private half. This is
// the exact dance the shithub server will perform after S41c — if the
// CLI's seal output isn't decryptable here, it won't be on the server
// either.
func TestSealAnonymousRoundTrip(t *testing.T) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub[:])

	plaintext := []byte("hunter2-correct-horse-battery-staple")
	ciphertextB64, err := SealAnonymous(pubB64, plaintext)
	if err != nil {
		t.Fatalf("SealAnonymous: %v", err)
	}
	cipherBytes, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	got, ok := box.OpenAnonymous(nil, cipherBytes, pub, priv)
	if !ok {
		t.Fatalf("OpenAnonymous: failed to decrypt")
	}
	if string(got) != string(plaintext) {
		t.Errorf("plaintext mismatch: got %q want %q", got, plaintext)
	}
}

func TestSealAnonymousRejectsBadKey(t *testing.T) {
	cases := map[string]string{
		"empty":          "",
		"not base64":     "!!!not base64!!!",
		"wrong key size": base64.StdEncoding.EncodeToString([]byte("too short")),
	}
	for name, key := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SealAnonymous(key, []byte("v")); err == nil {
				t.Errorf("expected error for %s key", name)
			}
		})
	}
}
