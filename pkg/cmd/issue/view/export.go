// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

// exporter projects a single *issues.Issue onto the gh-compatible JSON
// shape. Single-issue projection here intentionally mirrors `issue list`'s
// per-item shape so consumers don't have to special-case.
type exporter struct{}

func (exporter) Fields() []string { return exportableFields }

var exportableFields = []string{
	"assignees",
	"author",
	"body",
	"closedAt",
	"comments",
	"createdAt",
	"id",
	"labels",
	"locked",
	"milestone",
	"number",
	"pinned",
	"state",
	"stateReason",
	"title",
	"updatedAt",
	"url",
}

func (exporter) Filter(v any) (any, error) {
	i, ok := v.(*issues.Issue)
	if !ok {
		return nil, fmt.Errorf("issue view exporter: want *issues.Issue, got %T", v)
	}
	return projectIssue(*i), nil
}

// projectIssue is a small local copy of pkg/cmd/issue/list.projectIssue
// to avoid an import cycle. Kept in sync with that file by tests via the
// shared field name list.
func projectIssue(i issues.Issue) map[string]any {
	assignees := make([]map[string]any, 0, len(i.Assignees))
	for _, a := range i.Assignees {
		assignees = append(assignees, map[string]any{"login": a.Login})
	}
	labels := make([]map[string]any, 0, len(i.Labels))
	for _, l := range i.Labels {
		labels = append(labels, map[string]any{"name": l.Name, "color": l.Color})
	}
	var author any
	if i.User != nil {
		author = map[string]any{"login": i.User.Login}
	}
	var milestone any
	if i.Milestone != nil {
		milestone = map[string]any{"number": i.Milestone.Number, "title": i.Milestone.Title}
	}
	return map[string]any{
		"assignees":   assignees,
		"author":      author,
		"body":        i.Body,
		"closedAt":    i.ClosedAt,
		"comments":    i.Comments,
		"createdAt":   i.CreatedAt,
		"id":          i.ID,
		"labels":      labels,
		"locked":      i.Locked,
		"milestone":   milestone,
		"number":      i.Number,
		"pinned":      i.Pinned,
		"state":       i.State,
		"stateReason": i.StateReason,
		"title":       i.Title,
		"updatedAt":   i.UpdatedAt,
		"url":         i.HTMLURL,
	}
}
