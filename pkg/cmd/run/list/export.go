// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/actions"
)

type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"conclusion", "createdAt", "displayTitle", "event", "headBranch", "headSha",
		"id", "name", "number", "status", "updatedAt", "url", "workflowId",
	}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]actions.WorkflowRun)
	if !ok {
		return nil, fmt.Errorf("run list exporter: want []actions.WorkflowRun, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, r := range xs {
		out = append(out, map[string]any{
			"conclusion":   r.Conclusion,
			"createdAt":    r.CreatedAt,
			"displayTitle": r.DisplayTitle,
			"event":        r.Event,
			"headBranch":   r.HeadBranch,
			"headSha":      r.HeadSHA,
			"id":           r.ID,
			"name":         r.Name,
			"number":       r.RunNumber,
			"status":       r.Status,
			"updatedAt":    r.UpdatedAt,
			"url":          r.HTMLURL,
			"workflowId":   r.WorkflowID,
		})
	}
	return out, nil
}
