// SPDX-License-Identifier: AGPL-3.0-or-later

// Package keys owns the typed wire shape and helper client for
// shithub's /user/keys and /user/gpg_keys surfaces (S50 §11). Field
// names mirror GitHub's REST contract verbatim.
package keys

import "time"

// SSHKindAuthentication and SSHKindSigning are the two values gh accepts
// for `ssh-key add --type`. shithub's server may not honor the split
// yet (S07's user_ssh_keys table has no `kind` column today); the CLI
// always sends the value and lets the server decide.
const (
	SSHKindAuthentication = "authentication"
	SSHKindSigning        = "signing"
)

// SSHKey is one entry returned by /user/keys.
type SSHKey struct {
	ID int64 `json:"id"`
	// NodeID is the opaque base64-encoded `gid://shithub/SSHKey/{id}`
	// identifier (I7b audit-I25). gh-compat clients should prefer
	// NodeID over the sequential integer ID; the integer is kept for
	// one release cycle before the v0.2.0 strip.
	NodeID      string    `json:"node_id,omitempty"`
	Key         string    `json:"key"`
	Title       string    `json:"title"`
	Kind        string    `json:"kind,omitempty"` // "authentication" | "signing"
	URL         string    `json:"url,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Verified    bool      `json:"verified,omitempty"`
	ReadOnly    bool      `json:"read_only,omitempty"`
}

// SSHKeyInput is the body for POST /user/keys.
type SSHKeyInput struct {
	Title string `json:"title"`
	Key   string `json:"key"`
	Kind  string `json:"kind,omitempty"`
}

// GPGKey is one entry returned by /user/gpg_keys. The wire shape mirrors
// gh's response: a flat envelope plus an array of `Emails` and an array
// of `Subkeys`. Subkeys reuse the same shape recursively (one level
// deep is enough for daily-driver use).
type GPGKey struct {
	ID                int64      `json:"id"`
	KeyID             string     `json:"key_id"`
	PublicKey         string     `json:"public_key,omitempty"`
	Name              string     `json:"name,omitempty"`
	Emails            []GPGEmail `json:"emails,omitempty"`
	Subkeys           []GPGKey   `json:"subkeys,omitempty"`
	CanCertify        bool       `json:"can_certify,omitempty"`
	CanSign           bool       `json:"can_sign,omitempty"`
	CanEncryptComms   bool       `json:"can_encrypt_comms,omitempty"`
	CanEncryptStorage bool       `json:"can_encrypt_storage,omitempty"`
	CanAuthenticate   bool       `json:"can_authenticate,omitempty"`
	PrimaryKeyID      *int64     `json:"primary_key_id,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

// GPGEmail is one verified-email entry on a GPG key.
type GPGEmail struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified,omitempty"`
}

// GPGKeyInput is the body for POST /user/gpg_keys. shithub server
// (S50 §11) parses the armored block server-side; the CLI only forwards
// the bytes verbatim.
type GPGKeyInput struct {
	Name             string `json:"name,omitempty"`
	ArmoredPublicKey string `json:"armored_public_key"`
}
