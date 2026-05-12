// SPDX-License-Identifier: AGPL-3.0-or-later

// Package key holds light parsing helpers for public-key blobs. The
// single critical job: refuse to upload a private key by accident
// (audit-relevant — even gh ships a similar check). Parsing OpenSSH
// internals is out of scope; we just look at the first line.
package key

import (
	"errors"
	"strings"
)

// ErrPrivateKey is returned by DetectPublicKey when the candidate blob
// looks like a private key. Callers surface this verbatim so the user
// sees exactly why their upload was refused.
var ErrPrivateKey = errors.New("input looks like a private key; refusing to upload — re-run with the public key (typically the .pub file)")

// Kind identifies the public-key flavor we recognized.
type Kind int

const (
	// KindUnknown is the zero value; DetectPublicKey returns an error
	// before this is observable to callers.
	KindUnknown Kind = iota
	// KindSSH is an OpenSSH public-key line (`ssh-rsa AAAA…`, `ssh-ed25519 …`,
	// `ecdsa-sha2-… …`, `sk-ssh-ed25519@openssh.com …`).
	KindSSH
	// KindGPGArmor is an ASCII-armored OpenPGP public key block.
	KindGPGArmor
)

// privateMarkers are the leading tokens of known private-key formats.
// Any match triggers ErrPrivateKey. We compare against the first
// non-empty line, trimmed.
var privateMarkers = []string{
	"-----BEGIN OPENSSH PRIVATE KEY-----",
	"-----BEGIN RSA PRIVATE KEY-----",
	"-----BEGIN DSA PRIVATE KEY-----",
	"-----BEGIN EC PRIVATE KEY-----",
	"-----BEGIN PRIVATE KEY-----",           // PKCS#8
	"-----BEGIN ENCRYPTED PRIVATE KEY-----", // PKCS#8 encrypted
	"-----BEGIN PGP PRIVATE KEY BLOCK-----",
}

// sshPrefixes are the OpenSSH public-key type tokens we accept. The
// list is conservative — we don't try to enumerate every algorithm,
// just the ones gh / shithub server are known to accept. Unknown
// prefixes fall through to KindUnknown and an error.
var sshPrefixes = []string{
	"ssh-rsa ",
	"ssh-dss ",
	"ssh-ed25519 ",
	"ecdsa-sha2-nistp256 ",
	"ecdsa-sha2-nistp384 ",
	"ecdsa-sha2-nistp521 ",
	"sk-ssh-ed25519@openssh.com ",
	"sk-ecdsa-sha2-nistp256@openssh.com ",
}

// DetectPublicKey inspects blob and returns the recognized Kind plus an
// extracted comment (best-effort, used as a default --title hint). On a
// private-key marker it returns ErrPrivateKey verbatim. On an unknown
// format it returns a wrapped error so the user gets a clear "this
// doesn't look like a public key" message.
func DetectPublicKey(blob string) (Kind, string, error) {
	trimmed := strings.TrimSpace(blob)
	if trimmed == "" {
		return KindUnknown, "", errors.New("key: empty input")
	}
	firstLine := trimmed
	if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
		firstLine = strings.TrimSpace(trimmed[:i])
	}

	for _, m := range privateMarkers {
		if strings.HasPrefix(firstLine, m) {
			return KindUnknown, "", ErrPrivateKey
		}
	}

	if strings.HasPrefix(firstLine, "-----BEGIN PGP PUBLIC KEY BLOCK-----") {
		return KindGPGArmor, "", nil
	}

	for _, p := range sshPrefixes {
		if strings.HasPrefix(firstLine, p) {
			// The OpenSSH public-key format is `<type> <base64> [<comment>]`.
			// The comment, if any, is everything after the second space.
			rest := strings.TrimPrefix(firstLine, p)
			parts := strings.SplitN(rest, " ", 2)
			comment := ""
			if len(parts) == 2 {
				comment = strings.TrimSpace(parts[1])
			}
			return KindSSH, comment, nil
		}
	}
	return KindUnknown, "", errors.New("key: input doesn't look like a public SSH or GPG key (expected `ssh-rsa …`, `ssh-ed25519 …`, or an armored PGP block)")
}
