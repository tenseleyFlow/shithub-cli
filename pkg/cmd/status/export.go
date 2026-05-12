// SPDX-License-Identifier: AGPL-3.0-or-later

package status

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/search"
)

// exporter projects the Dashboard onto the gh-compatible JSON shape.
// Field names match the JSON struct tags on Dashboard so callers can
// round-trip safely.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{"user", "assigned_issues", "assigned_prs", "review_requests", "mentions"}
}

func (exporter) Filter(v any) (any, error) {
	d, ok := v.(Dashboard)
	if !ok {
		return nil, fmt.Errorf("status exporter: want Dashboard, got %T", v)
	}
	return map[string]any{
		"user":            d.User,
		"assigned_issues": issuesProj(d.AssignedIssues),
		"assigned_prs":    prsProj(d.AssignedPRs),
		"review_requests": prsProj(d.ReviewRequests),
		"mentions":        issuesProj(d.Mentions),
	}, nil
}

func issuesProj(items []search.IssueItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		repo := ""
		if it.Repository != nil {
			repo = it.Repository.FullName
		}
		out = append(out, map[string]any{
			"number":     it.Number,
			"title":      it.Title,
			"state":      it.State,
			"repository": repo,
			"updatedAt":  it.UpdatedAt,
			"url":        it.HTMLURL,
		})
	}
	return out
}

func prsProj(items []search.PRItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, pr := range items {
		repo := ""
		if pr.Repository != nil {
			repo = pr.Repository.FullName
		}
		out = append(out, map[string]any{
			"number":     pr.Number,
			"title":      pr.Title,
			"state":      pr.State,
			"draft":      pr.Draft,
			"repository": repo,
			"updatedAt":  pr.UpdatedAt,
			"url":        pr.HTMLURL,
		})
	}
	return out
}
