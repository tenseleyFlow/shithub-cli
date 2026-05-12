// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

// exporter projects each PR onto the gh-compatible JSON shape.
type exporter struct{}

func (exporter) Fields() []string { return exportableFields }

// ExportableFields returns the catalog used by both `pr list --json`
// and `pr view --json`. Promoted so the view package can stay in sync
// without duplicating the slice.
func ExportableFields() []string {
	return append([]string(nil), exportableFields...)
}

var exportableFields = []string{
	"author",
	"baseRefName",
	"body",
	"closedAt",
	"comments",
	"createdAt",
	"draft",
	"headRefName",
	"id",
	"isDraft",
	"labels",
	"merged",
	"mergedAt",
	"number",
	"reviewDecision",
	"state",
	"title",
	"updatedAt",
	"url",
}

func (exporter) Filter(v any) (any, error) {
	list, ok := v.([]pulls.PR)
	if !ok {
		return nil, fmt.Errorf("pr list exporter: want []pulls.PR, got %T", v)
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, ProjectPR(p))
	}
	return out, nil
}

// ProjectPR is shared with `pr view` so single-PR and listing exports
// emit identical field names.
func ProjectPR(p pulls.PR) map[string]any {
	var author any
	if p.User != nil {
		author = map[string]any{"login": p.User.Login}
	}
	labels := make([]map[string]any, 0, len(p.Labels))
	for _, l := range p.Labels {
		labels = append(labels, map[string]any{"name": l.Name, "color": l.Color})
	}
	return map[string]any{
		"author":         author,
		"baseRefName":    p.Base.Ref,
		"body":           p.Body,
		"closedAt":       p.ClosedAt,
		"comments":       p.Comments,
		"createdAt":      p.CreatedAt,
		"draft":          p.Draft,
		"headRefName":    p.Head.Ref,
		"id":             p.ID,
		"isDraft":        p.Draft,
		"labels":         labels,
		"merged":         p.Merged,
		"mergedAt":       p.MergedAt,
		"number":         p.Number,
		"reviewDecision": p.ReviewDecision,
		"state":          p.State,
		"title":          p.Title,
		"updatedAt":      p.UpdatedAt,
		"url":            p.HTMLURL,
	}
}
