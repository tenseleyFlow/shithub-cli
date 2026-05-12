// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{"createdAt", "id", "name", "path", "state", "updatedAt", "url"}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]actions.Workflow)
	if !ok {
		return nil, fmt.Errorf("workflow list exporter: want []actions.Workflow, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, w := range xs {
		out = append(out, map[string]any{
			"createdAt": w.CreatedAt,
			"id":        w.ID,
			"name":      w.Name,
			"path":      w.Path,
			"state":     w.State,
			"updatedAt": w.UpdatedAt,
			"url":       w.HTMLURL,
		})
	}
	return out, nil
}
