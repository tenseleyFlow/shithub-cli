// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{"createdAt", "fingerprint", "id", "kind", "key", "readOnly", "title", "url", "verified"}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]keys.SSHKey)
	if !ok {
		return nil, fmt.Errorf("ssh-key list exporter: want []keys.SSHKey, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, k := range xs {
		kind := k.Kind
		if kind == "" {
			kind = keys.SSHKindAuthentication
		}
		out = append(out, map[string]any{
			"createdAt":   k.CreatedAt,
			"fingerprint": k.Fingerprint,
			"id":          k.ID,
			"kind":        kind,
			"key":         k.Key,
			"readOnly":    k.ReadOnly,
			"title":       k.Title,
			"url":         k.URL,
			"verified":    k.Verified,
		})
	}
	return out, nil
}
