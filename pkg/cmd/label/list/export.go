// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/labels"
)

// exporter projects each label onto the gh-compatible JSON shape.
type exporter struct{}

func (exporter) Fields() []string { return ExportableFields() }

// ExportableFields returns the catalog used by `label list --json`.
// Promoted so the (future) view package can stay in sync.
func ExportableFields() []string {
	return []string{
		"color",
		"createdAt",
		"default",
		"description",
		"id",
		"name",
		"updatedAt",
		"url",
	}
}

func (exporter) Filter(v any) (any, error) {
	list, ok := v.([]labels.Label)
	if !ok {
		return nil, fmt.Errorf("label list exporter: want []labels.Label, got %T", v)
	}
	out := make([]map[string]any, 0, len(list))
	for _, l := range list {
		out = append(out, ProjectLabel(l))
	}
	return out, nil
}

// ProjectLabel exposes the single-label projection so other label
// subcommands (create/edit) can render --json output without duplicating
// the field list.
func ProjectLabel(l labels.Label) map[string]any {
	return map[string]any{
		"color":       l.Color,
		"createdAt":   l.CreatedAt,
		"default":     l.Default,
		"description": l.Description,
		"id":          l.ID,
		"name":        l.Name,
		"updatedAt":   l.UpdatedAt,
		"url":         l.URL,
	}
}
