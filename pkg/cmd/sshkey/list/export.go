// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/keys"
)

type exporter struct{}

func (exporter) Fields() []string {
	// I7b (audit-I25): `nodeId` lives alongside the sequential `id` for
	// one release cycle. New scripts should pick `nodeId`; `id` will be
	// stripped at v0.2.0.
	return []string{"createdAt", "fingerprint", "id", "kind", "key", "nodeId", "readOnly", "title", "url", "verified"}
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
			"nodeId":      k.NodeID,
			"readOnly":    k.ReadOnly,
			"title":       k.Title,
			"url":         k.URL,
			"verified":    k.Verified,
		})
	}
	return out, nil
}
