// SPDX-License-Identifier: AGPL-3.0-or-later

// Package crypt holds CLI-side cryptographic primitives that the rest
// of the binary leans on. Today: libsodium-compatible sealed-box for
// Actions secrets (per gh / GitHub Actions spec). Anything we add here
// must be byte-compatible with what the shithub server decrypts with.
package crypt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/box"
)

// SealedBoxKeySize is the wire length of a curve25519 public key.
const SealedBoxKeySize = 32

// SealAnonymous encrypts plaintext under a libsodium-compatible
// "anonymous sealed box" against pubKeyB64 (base64 of a 32-byte
// curve25519 public key). The output is base64 of:
//
//	ephemeral_public_key (32 bytes) || ciphertext || mac
//
// which is what gh produces and what shithub's server-side decryption
// expects. The server holds the matching private key in its KMS slot
// per the S41c migration plan.
func SealAnonymous(pubKeyB64 string, plaintext []byte) (string, error) {
	if pubKeyB64 == "" {
		return "", errors.New("crypt: empty public key")
	}
	keyBytes, err := base64.StdEncoding.DecodeString(pubKeyB64)
	if err != nil {
		return "", fmt.Errorf("crypt: decode public key: %w", err)
	}
	if len(keyBytes) != SealedBoxKeySize {
		return "", fmt.Errorf("crypt: public key must be %d bytes, got %d", SealedBoxKeySize, len(keyBytes))
	}
	var pubKey [SealedBoxKeySize]byte
	copy(pubKey[:], keyBytes)

	sealed, err := box.SealAnonymous(nil, plaintext, &pubKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("crypt: seal: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}
