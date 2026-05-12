// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/orgs"
)

// exporter projects each org onto the gh-compatible JSON shape for
// `--json`.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"avatarUrl", "createdAt", "description", "id", "isVerified",
		"login", "members", "name", "publicRepos", "role", "suspended", "url",
	}
}

func (exporter) Filter(v any) (any, error) {
	xs, ok := v.([]orgs.Org)
	if !ok {
		return nil, fmt.Errorf("org list exporter: want []orgs.Org, got %T", v)
	}
	out := make([]map[string]any, 0, len(xs))
	for _, o := range xs {
		out = append(out, map[string]any{
			"avatarUrl":   o.AvatarURL,
			"createdAt":   o.CreatedAt,
			"description": o.Description,
			"id":          o.ID,
			"isVerified":  o.IsVerified,
			"login":       o.Login,
			"members":     o.MembersCount,
			"name":        o.Name,
			"publicRepos": o.PublicRepos,
			"role":        o.Role,
			"suspended":   o.Suspended,
			"url":         o.HTMLURL,
		})
	}
	return out, nil
}
