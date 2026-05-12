// SPDX-License-Identifier: AGPL-3.0-or-later

// Package labels owns the typed wire shape and helper client for
// shithub's /repos/{o}/{r}/labels surface. Field tags mirror GitHub's
// REST contract verbatim.
package labels

import "time"

// Label is the canonical envelope returned by every CRUD endpoint.
// Color is server-canonical six-digit hex without the leading '#'.
type Label struct {
	ID          int64     `json:"id,omitempty"`
	NodeID      string    `json:"node_id,omitempty"`
	URL         string    `json:"url,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Color       string    `json:"color,omitempty"`
	Default     bool      `json:"default,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

// CreateInput is the body for POST /labels.
type CreateInput struct {
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

// EditInput is the body for PATCH /labels/{name}. NewName triggers a
// rename when set; other fields update in place. Pointers (rather than
// omitempty strings) let callers explicitly set "" to clear description.
type EditInput struct {
	NewName     *string `json:"new_name,omitempty"`
	Color       *string `json:"color,omitempty"`
	Description *string `json:"description,omitempty"`
}

// IsEmpty reports whether the patch carries any changes.
func (e EditInput) IsEmpty() bool {
	return e.NewName == nil && e.Color == nil && e.Description == nil
}

// ListOptions captures the filter/sort knobs accepted by GET /labels.
// The shithub endpoint mirrors GitHub's surface; --search filters
// client-side because the upstream API doesn't take a query parameter
// for labels.
type ListOptions struct {
	Sort      string // "name" | "created"
	Direction string // "asc" | "desc"
	PerPage   int
	Limit     int
}
