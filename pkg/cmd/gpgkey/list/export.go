// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"canAuthenticate", "canCertify", "canEncryptComms", "canEncryptStorage",
		"canSign", "createdAt", "emails", "expiresAt", "id", "keyId", "publicKey",
	}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]keys.GPGKey)
	if !ok {
		return nil, fmt.Errorf("gpg-key list exporter: want []keys.GPGKey, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, k := range xs {
		emails := make([]map[string]any, 0, len(k.Emails))
		for _, e := range k.Emails {
			emails = append(emails, map[string]any{"email": e.Email, "verified": e.Verified})
		}
		out = append(out, map[string]any{
			"canAuthenticate":   k.CanAuthenticate,
			"canCertify":        k.CanCertify,
			"canEncryptComms":   k.CanEncryptComms,
			"canEncryptStorage": k.CanEncryptStorage,
			"canSign":           k.CanSign,
			"createdAt":         k.CreatedAt,
			"emails":            emails,
			"expiresAt":         k.ExpiresAt,
			"id":                k.ID,
			"keyId":             k.KeyID,
			"publicKey":         k.PublicKey,
		})
	}
	return out, nil
}
