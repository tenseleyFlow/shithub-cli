// SPDX-License-Identifier: AGPL-3.0-or-later

package issues

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

// exporter projects issue results onto the JSON shape used by `issue
// view --json`. The PR subcommand carries its own exporter with the
// PR-extra fields.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"assignees", "author", "body", "closedAt", "comments", "createdAt",
		"id", "isLocked", "labels", "milestone", "number", "repository",
		"state", "stateReason", "title", "updatedAt", "url",
	}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]issues.Issue)
	if !ok {
		return nil, fmt.Errorf("search issues exporter: want []issues.Issue, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, it := range xs {
		var author any
		if it.User != nil {
			author = map[string]any{"login": it.User.Login, "id": it.User.ID}
		}
		assignees := make([]map[string]any, 0, len(it.Assignees))
		for _, u := range it.Assignees {
			assignees = append(assignees, map[string]any{"login": u.Login, "id": u.ID})
		}
		labels := make([]map[string]any, 0, len(it.Labels))
		for _, l := range it.Labels {
			labels = append(labels, map[string]any{"name": l.Name, "color": l.Color})
		}
		var milestone any
		if it.Milestone != nil {
			milestone = map[string]any{"title": it.Milestone.Title, "number": it.Milestone.Number}
		}
		var repo any
		if it.Repository != nil {
			repo = map[string]any{"fullName": it.Repository.FullName}
		}
		out = append(out, map[string]any{
			"assignees":   assignees,
			"author":      author,
			"body":        it.Body,
			"closedAt":    it.ClosedAt,
			"comments":    it.Comments,
			"createdAt":   it.CreatedAt,
			"id":          it.ID,
			"isLocked":    it.Locked,
			"labels":      labels,
			"milestone":   milestone,
			"number":      it.Number,
			"repository":  repo,
			"state":       it.State,
			"stateReason": it.StateReason,
			"title":       it.Title,
			"updatedAt":   it.UpdatedAt,
			"url":         it.HTMLURL,
		})
	}
	return out, nil
}
