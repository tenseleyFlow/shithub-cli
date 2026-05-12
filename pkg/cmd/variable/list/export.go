// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/secrets"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{"name", "value", "createdAt", "updatedAt", "visibility", "selectedRepositoriesUrl"}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]secrets.Variable)
	if !ok {
		return nil, fmt.Errorf("variable list exporter: want []secrets.Variable, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, vv := range xs {
		out = append(out, map[string]any{
			"name":                    vv.Name,
			"value":                   vv.Value,
			"createdAt":               vv.CreatedAt,
			"updatedAt":               vv.UpdatedAt,
			"visibility":              vv.Visibility,
			"selectedRepositoriesUrl": vv.SelectedRepositoriesURL,
		})
	}
	return out, nil
}
