// SPDX-License-Identifier: AGPL-3.0-or-later

// Package secrets owns the typed wire shape and helper client for
// shithub's GitHub-Actions-compatible /actions/secrets + /actions/variables
// surface (shithub S41c / S50). Repo-scoped endpoints are live; the
// org-scoped endpoints and sealed-box public-key endpoint are pending
// on the server. Callers handle 404 from public-key as the explicit
// "send plaintext until the migration ships" signal.
package secrets

import "time"

// Secret is one entry from /actions/secrets. The API never returns the
// secret's value — only the metadata. (For org-scoped secrets the
// visibility/selected_repos fields are populated.)
type Secret struct {
	Name                    string    `json:"name"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	Visibility              string    `json:"visibility,omitempty"`
	SelectedRepositoriesURL string    `json:"selected_repositories_url,omitempty"`
}

// SecretsResponse is the paginated list envelope.
type SecretsResponse struct {
	TotalCount int      `json:"total_count"`
	Secrets    []Secret `json:"secrets"`
}

// Variable is one entry from /actions/variables. Unlike secrets, the
// value is returned in plaintext on read.
type Variable struct {
	Name                    string    `json:"name"`
	Value                   string    `json:"value"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	Visibility              string    `json:"visibility,omitempty"`
	SelectedRepositoriesURL string    `json:"selected_repositories_url,omitempty"`
}

// VariablesResponse is the paginated list envelope.
type VariablesResponse struct {
	TotalCount int        `json:"total_count"`
	Variables  []Variable `json:"variables"`
}

// PublicKey is the response from /actions/secrets/public-key. The server
// returns a base64-encoded 32-byte curve25519 public key and an opaque
// key_id the client must echo back in subsequent PUT calls so the server
// knows which private key to decrypt with.
type PublicKey struct {
	KeyID string `json:"key_id"`
	Key   string `json:"key"` // base64
}

// SetSecretInput is the body shape for PUT /actions/secrets/{name}.
// EncryptedValue is the base64 ciphertext from nacl/box.SealAnonymous;
// when the server doesn't yet support sealed-box (S41c migration
// pending), CLI falls back to setting PlaintextValue and surfaces a
// warning. KeyID must match the public key the ciphertext was sealed
// against. Visibility/SelectedRepositoryIDs are org-scope only.
type SetSecretInput struct {
	EncryptedValue        string  `json:"encrypted_value,omitempty"`
	PlaintextValue        string  `json:"value,omitempty"`
	KeyID                 string  `json:"key_id,omitempty"`
	Visibility            string  `json:"visibility,omitempty"`
	SelectedRepositoryIDs []int64 `json:"selected_repository_ids,omitempty"`
}

// SetVariableInput is the body shape for POST/PATCH /actions/variables.
// Variables ship plaintext; no encryption involved.
type SetVariableInput struct {
	Name                  string  `json:"name,omitempty"`
	Value                 string  `json:"value"`
	Visibility            string  `json:"visibility,omitempty"`
	SelectedRepositoryIDs []int64 `json:"selected_repository_ids,omitempty"`
}
