// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{"createdAt", "id", "key", "lastAccessedAt", "ref", "sizeInBytes", "version"}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]actions.Cache)
	if !ok {
		return nil, fmt.Errorf("cache list exporter: want []actions.Cache, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, c := range xs {
		out = append(out, map[string]any{
			"createdAt":      c.CreatedAt,
			"id":             c.ID,
			"key":            c.Key,
			"lastAccessedAt": c.LastAccessedAt,
			"ref":            c.Ref,
			"sizeInBytes":    c.SizeInBytes,
			"version":        c.Version,
		})
	}
	return out, nil
}
