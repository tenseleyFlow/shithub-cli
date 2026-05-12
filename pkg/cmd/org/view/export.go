// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/orgs"
)

// exporter projects the Org envelope onto the JSON shape gh uses for
// `org view --json`.
type exporter struct{}

func (exporter) Fields() []string {
	return []string{
		"avatarUrl", "blog", "company", "createdAt", "description",
		"email", "id", "isVerified", "location", "login", "members",
		"name", "publicRepos", "suspended", "twitterUsername", "updatedAt", "url",
	}
}

func (exporter) Filter(v any) (any, error) {
	o, ok := v.(orgs.Org)
	if !ok {
		return nil, fmt.Errorf("org view exporter: want orgs.Org, got %T", v)
	}
	return map[string]any{
		"avatarUrl":       o.AvatarURL,
		"blog":            o.Blog,
		"company":         o.Company,
		"createdAt":       o.CreatedAt,
		"description":     o.Description,
		"email":           o.Email,
		"id":              o.ID,
		"isVerified":      o.IsVerified,
		"location":        o.Location,
		"login":           o.Login,
		"members":         o.MembersCount,
		"name":            o.Name,
		"publicRepos":     o.PublicRepos,
		"suspended":       o.Suspended,
		"twitterUsername": o.TwitterUser,
		"updatedAt":       o.UpdatedAt,
		"url":             o.HTMLURL,
	}, nil
}
