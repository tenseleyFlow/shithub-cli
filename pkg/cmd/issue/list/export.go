// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

// exporter projects each issue onto the gh-compatible JSON shape.
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
	"nodeId",
	"number",
	"pinned",
	"repository",
	"state",
	"stateReason",
	"title",
	"updatedAt",
	"url",
}

func (exporter) Filter(v any) (any, error) {
	list, ok := v.([]issues.Issue)
	if !ok {
		return nil, fmt.Errorf("issue list exporter: want []issues.Issue, got %T", v)
	}
	out := make([]map[string]any, 0, len(list))
	for _, i := range list {
		out = append(out, projectIssue(i))
	}
	return out, nil
}

// projectIssue is shared with `issue view` so single-issue and listing
// exports emit identical field names.
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
	var repo any
	if i.Repository != nil {
		repo = map[string]any{"nameWithOwner": i.Repository.FullName, "name": i.Repository.Name}
	}
	return map[string]any{
		"assignees": assignees,
		"author":    author,
		"body":      i.Body,
		"closedAt":  i.ClosedAt,
		"comments":  i.Comments,
		"createdAt": i.CreatedAt,
		"id":        i.ID,
		"labels":    labels,
		"locked":    i.Locked,
		"milestone": milestone,
		// I7b (audit-I25): opaque opaque-id alongside the sequential id.
		"nodeId":      i.NodeID,
		"number":      i.Number,
		"pinned":      i.Pinned,
		"repository":  repo,
		"state":       i.State,
		"stateReason": i.StateReason,
		"title":       i.Title,
		"updatedAt":   i.UpdatedAt,
		"url":         i.HTMLURL,
	}
}
