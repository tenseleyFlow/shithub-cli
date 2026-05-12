// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{"name", "createdAt", "updatedAt", "visibility", "selectedRepositoriesUrl"}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]secrets.Secret)
	if !ok {
		return nil, fmt.Errorf("secret list exporter: want []secrets.Secret, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, s := range xs {
		out = append(out, map[string]any{
			"name":                    s.Name,
			"createdAt":               s.CreatedAt,
			"updatedAt":               s.UpdatedAt,
			"visibility":              s.Visibility,
			"selectedRepositoriesUrl": s.SelectedRepositoriesURL,
		})
	}
	return out, nil
}
