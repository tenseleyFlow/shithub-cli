// SPDX-License-Identifier: AGPL-3.0-or-later

package repos

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

// exporter projects each repo onto the gh-compatible JSON shape for
// `--json`. Mirrors `repo list`'s vocabulary so the two commands stay
// interchangeable when scripting.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"archived", "createdAt", "defaultBranch", "description", "fork",
		"forks", "fullName", "isPrivate", "language", "name", "owner",
		"pushedAt", "stargazers", "topics", "updatedAt", "url", "visibility",
	}
}

func (exporter) Filter(v any) (any, error) {
	rs, ok := v.([]repos.Repo)
	if !ok {
		return nil, fmt.Errorf("search repos exporter: want []repos.Repo, got %T", v)
	}
	out := make([]map[string]any, 0, len(rs))
	for _, r := range rs {
		out = append(out, map[string]any{
			"archived":      r.Archived,
			"createdAt":     r.CreatedAt,
			"defaultBranch": r.DefaultBranch,
			"description":   r.Description,
			"fork":          r.Fork,
			"forks":         r.Forks,
			"fullName":      r.FullName,
			"isPrivate":     r.Private,
			"language":      r.Language,
			"name":          r.Name,
			"owner":         map[string]any{"login": r.Owner.Login, "type": r.Owner.Type},
			"pushedAt":      r.PushedAt,
			"stargazers":    r.Stargazers,
			"topics":        r.Topics,
			"updatedAt":     r.UpdatedAt,
			"url":           r.HTMLURL,
			"visibility":    visibilityField(&r),
		})
	}
	return out, nil
}

func visibilityField(r *repos.Repo) string {
	if r.Visibility != "" {
		return r.Visibility
	}
	if r.Private {
		return "private"
	}
	return "public"
}
