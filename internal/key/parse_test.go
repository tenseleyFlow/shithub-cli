// SPDX-License-Identifier: AGPL-3.0-or-later

package key

import (
	"errors"
	"testing"
)

func TestDetectPublicKeySSH(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample mf@laptop\n": "mf@laptop",
		"ssh-rsa AAAAB3NzaC1yc2EAAAA…":                             "",
		"ecdsa-sha2-nistp256 AAAAE2VjZHNh… work-key":               "work-key",
		"sk-ssh-ed25519@openssh.com AAAAC3Nz… yubikey":             "yubikey",
	}
	for blob, wantComment := range cases {
		k, comment, err := DetectPublicKey(blob)
		if err != nil {
			t.Errorf("DetectPublicKey(%q): %v", blob, err)
			continue
		}
		if k != KindSSH {
			t.Errorf("kind for %q = %d", blob, k)
		}
		if comment != wantComment {
			t.Errorf("comment for %q = %q; want %q", blob, comment, wantComment)
		}
	}
}

func TestDetectPublicKeyGPGArmor(t *testing.T) {
	t.Parallel()
	blob := "-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmQENBF…\n-----END PGP PUBLIC KEY BLOCK-----\n"
	k, _, err := DetectPublicKey(blob)
	if err != nil {
		t.Fatalf("DetectPublicKey: %v", err)
	}
	if k != KindGPGArmor {
		t.Errorf("kind = %d", k)
	}
}

// TestDetectPublicKeyRefusesPrivate is the load-bearing safety check —
// upload-by-mistake is exactly what this guard prevents.
func TestDetectPublicKeyRefusesPrivate(t *testing.T) {
	t.Parallel()
	bad := []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk…\n-----END OPENSSH PRIVATE KEY-----",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA…",
		"-----BEGIN PGP PRIVATE KEY BLOCK-----\n",
		"-----BEGIN PRIVATE KEY-----\n",           // PKCS#8
		"-----BEGIN ENCRYPTED PRIVATE KEY-----\n", // PKCS#8 encrypted
	}
	for _, blob := range bad {
		_, _, err := DetectPublicKey(blob)
		if !errors.Is(err, ErrPrivateKey) {
			t.Errorf("DetectPublicKey(%q): want ErrPrivateKey, got %v", blob, err)
		}
	}
}

func TestDetectPublicKeyRejectsGarbage(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "    ", "hello world\n", "not-a-key"} {
		if _, _, err := DetectPublicKey(in); err == nil {
			t.Errorf("DetectPublicKey(%q): want error", in)
		}
	}
}
