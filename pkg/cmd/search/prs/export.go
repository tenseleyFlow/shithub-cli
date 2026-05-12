// SPDX-License-Identifier: AGPL-3.0-or-later

package prs

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

// exporter projects PR results into the gh-compatible JSON shape. PR
// extras (draft / merged / review state) are first-class fields.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"additions", "assignees", "author", "baseRefName", "body",
		"changedFiles", "closedAt", "comments", "commits", "createdAt",
		"deletions", "headRefName", "id", "isDraft", "isLocked", "labels",
		"mergeable", "mergedAt", "milestone", "number", "reviewDecision",
		"state", "title", "updatedAt", "url",
	}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]pulls.PR)
	if !ok {
		return nil, fmt.Errorf("search prs exporter: want []pulls.PR, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, pr := range xs {
		var author any
		if pr.User != nil {
			author = map[string]any{"login": pr.User.Login, "id": pr.User.ID}
		}
		assignees := make([]map[string]any, 0, len(pr.Assignees))
		for _, u := range pr.Assignees {
			assignees = append(assignees, map[string]any{"login": u.Login, "id": u.ID})
		}
		labels := make([]map[string]any, 0, len(pr.Labels))
		for _, l := range pr.Labels {
			labels = append(labels, map[string]any{"name": l.Name, "color": l.Color})
		}
		var milestone any
		if pr.Milestone != nil {
			milestone = map[string]any{"title": pr.Milestone.Title, "number": pr.Milestone.Number}
		}
		var mergeable any
		if pr.Mergeable != nil {
			mergeable = *pr.Mergeable
		}
		out = append(out, map[string]any{
			"additions":      pr.Additions,
			"assignees":      assignees,
			"author":         author,
			"baseRefName":    pr.Base.Ref,
			"body":           pr.Body,
			"changedFiles":   pr.ChangedFiles,
			"closedAt":       pr.ClosedAt,
			"comments":       pr.Comments,
			"commits":        pr.Commits,
			"createdAt":      pr.CreatedAt,
			"deletions":      pr.Deletions,
			"headRefName":    pr.Head.Ref,
			"id":             pr.ID,
			"isDraft":        pr.Draft,
			"isLocked":       pr.Locked,
			"labels":         labels,
			"mergeable":      mergeable,
			"mergedAt":       pr.MergedAt,
			"milestone":      milestone,
			"number":         pr.Number,
			"reviewDecision": pr.ReviewDecision,
			"state":          pr.State,
			"title":          pr.Title,
			"updatedAt":      pr.UpdatedAt,
			"url":            pr.HTMLURL,
		})
	}
	return out, nil
}
