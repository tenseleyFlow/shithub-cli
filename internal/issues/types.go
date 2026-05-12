// SPDX-License-Identifier: AGPL-3.0-or-later

// Package issues owns the typed wire shape and helper client for shithub's
// /api/v1/repos/{owner}/{repo}/issues surface (and the /issues/comments
// sub-endpoints). Field tags mirror the gh / GitHub REST contract so
// shithub's S50 §3 implementation can reuse the same envelopes.
package issues

import (
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Issue is the canonical envelope. Fields covering features shithub doesn't
// implement yet (PullRequest, ActiveLockReason, StateReason) are typed but
// optional via omitempty so they round-trip cleanly when the server starts
// returning them.
type Issue struct {
	ID               int64      `json:"id"`
	NodeID           string     `json:"node_id,omitempty"`
	Number           int        `json:"number"`
	Title            string     `json:"title"`
	Body             string     `json:"body,omitempty"`
	State            string     `json:"state"` // "open" | "closed"
	StateReason      string     `json:"state_reason,omitempty"`
	User             *api.User  `json:"user,omitempty"`
	Assignees        []api.User `json:"assignees,omitempty"`
	Labels           []Label    `json:"labels,omitempty"`
	Milestone        *Milestone `json:"milestone,omitempty"`
	Comments         int        `json:"comments"`
	Locked           bool       `json:"locked,omitempty"`
	ActiveLockReason string     `json:"active_lock_reason,omitempty"`
	Pinned           bool       `json:"pinned,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ClosedAt         *time.Time `json:"closed_at,omitempty"`
	ClosedBy         *api.User  `json:"closed_by,omitempty"`
	HTMLURL          string     `json:"html_url"`
	Repository       *RepoRef   `json:"repository,omitempty"`
	PullRequest      *struct{}  `json:"pull_request,omitempty"` // presence marker
}

// IsPullRequest reports whether the issue envelope is actually a PR. gh
// uses the same /issues endpoint for both and disambiguates by the
// pull_request field; we copy the convention.
func (i Issue) IsPullRequest() bool { return i.PullRequest != nil }

// Label is the colored tag attached to issues. Color is server-canonical
// six-digit hex without the leading '#'.
type Label struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Color       string `json:"color,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// Milestone is the optional grouping affordance.
type Milestone struct {
	ID           int64      `json:"id"`
	Number       int        `json:"number"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	State        string     `json:"state,omitempty"`
	OpenIssues   int        `json:"open_issues,omitempty"`
	ClosedIssues int        `json:"closed_issues,omitempty"`
	DueOn        *time.Time `json:"due_on,omitempty"`
}

// Comment is one entry in an issue's discussion thread.
type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      *api.User `json:"user,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	HTMLURL   string    `json:"html_url,omitempty"`
}

// RepoRef is the minimal repo envelope returned inside cross-repo issue
// listings (e.g., `issue status`). Full Repo lives in internal/repos; we
// duplicate the few fields we need here to avoid an import cycle.
type RepoRef struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url,omitempty"`
	Private  bool   `json:"private,omitempty"`
}

// CreateInput is the body for POST /issues.
type CreateInput struct {
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Milestone *int     `json:"milestone,omitempty"`
}

// EditInput is the body for PATCH /issues/{n}. Pointers for fields that
// have a meaningful zero (state/state_reason) so callers can clear them
// without omission ambiguity.
type EditInput struct {
	Title       *string  `json:"title,omitempty"`
	Body        *string  `json:"body,omitempty"`
	State       *string  `json:"state,omitempty"`
	StateReason *string  `json:"state_reason,omitempty"`
	Assignees   []string `json:"assignees,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Milestone   *int     `json:"milestone,omitempty"` // explicit null clears
}

// IsEmpty reports whether the patch carries any changes. Used by `issue
// edit` to short-circuit when only delta-style flags (--add-label etc.)
// were given — those go through ListLabels/ReplaceLabels instead.
func (e EditInput) IsEmpty() bool {
	return e.Title == nil && e.Body == nil && e.State == nil && e.StateReason == nil &&
		e.Assignees == nil && e.Labels == nil && e.Milestone == nil
}

// CommentInput is the body for POST /issues/{n}/comments and the PATCH
// variant on a single comment.
type CommentInput struct {
	Body string `json:"body"`
}

// LockInput is the body for PUT /issues/{n}/lock. Reason is optional and
// must be one of {off_topic, too_heated, resolved, spam} when set.
type LockInput struct {
	Reason string `json:"lock_reason,omitempty"`
}

// ListOptions captures the filter knobs accepted by GET /issues.
type ListOptions struct {
	State     string // "open" | "closed" | "all"
	Labels    []string
	Sort      string // "created" | "updated" | "comments"
	Direction string // "asc" | "desc"
	Since     string // RFC3339
	Author    string
	Assignee  string
	Mentioned string
	Milestone string
	PerPage   int
	Limit     int // soft cap on total items
}
