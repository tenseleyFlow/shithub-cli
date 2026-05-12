// SPDX-License-Identifier: AGPL-3.0-or-later

package code

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/search"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{"name", "path", "repository", "sha", "textMatches", "url"}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]search.CodeItem)
	if !ok {
		return nil, fmt.Errorf("search code exporter: want []search.CodeItem, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, it := range xs {
		var repo any
		if it.Repository != nil {
			repo = map[string]any{"fullName": it.Repository.FullName}
		}
		out = append(out, map[string]any{
			"name":        it.Name,
			"path":        it.Path,
			"repository":  repo,
			"sha":         it.SHA,
			"textMatches": it.TextMatches,
			"url":         it.HTMLURL,
		})
	}
	return out, nil
}
