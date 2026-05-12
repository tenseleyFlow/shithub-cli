// SPDX-License-Identifier: AGPL-3.0-or-later

// Package orgs owns the typed wire shape and helper client for
// shithub's /orgs surface (S50 §7). Field tags mirror GitHub's REST
// contract verbatim so the same struct works against either host.
package orgs

import "time"

// Org is the canonical organization envelope returned by every read
// endpoint (list, view). The full set of fields is populated by
// /orgs/{org}; list endpoints return the minimal subset (no
// description / counts) and leave the rest at zero values.
type Org struct {
	ID          int64  `json:"id"`
	Login       string `json:"login"`
	NodeID      string `json:"node_id,omitempty"`
	URL         string `json:"url,omitempty"`
	HTMLURL     string `json:"html_url,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Description string `json:"description,omitempty"`

	Name        string `json:"name,omitempty"`
	Company     string `json:"company,omitempty"`
	Blog        string `json:"blog,omitempty"`
	Location    string `json:"location,omitempty"`
	Email       string `json:"email,omitempty"`
	TwitterUser string `json:"twitter_username,omitempty"`

	IsVerified      bool `json:"is_verified,omitempty"`
	HasOrgProjects  bool `json:"has_organization_projects,omitempty"`
	HasRepoProjects bool `json:"has_repository_projects,omitempty"`

	PublicRepos   int `json:"public_repos,omitempty"`
	PublicGists   int `json:"public_gists,omitempty"`
	Followers     int `json:"followers,omitempty"`
	Following     int `json:"following,omitempty"`
	Collaborators int `json:"collaborators,omitempty"`
	MembersCount  int `json:"members,omitempty"` // server-side count when cheap

	Type string `json:"type,omitempty"` // "Organization"

	// Role is server-derived: "admin" (owner) or "member". Only present
	// on /user/orgs responses — the public /orgs/{org} endpoint omits it
	// because the request isn't authenticated as a member.
	Role string `json:"role,omitempty"`

	// Suspended is a shithub-specific surface; gh's API doesn't echo it.
	// Lets `org list` mark suspended orgs with a badge.
	Suspended bool `json:"suspended,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// ListOptions drives pagination for the list endpoints. PerPage is
// clamped to [1, 100] by the client; Limit caps the total across pages.
type ListOptions struct {
	PerPage int
	Limit   int
}
