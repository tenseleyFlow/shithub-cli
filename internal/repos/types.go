// SPDX-License-Identifier: AGPL-3.0-or-later

// Package repos owns the typed wire shape and helper client for shithub's
// /api/v1/repos surface. Commands (pkg/cmd/repo/...) hold the UX; this
// package hides the JSON shapes so they don't leak. The struct names and
// field tags mirror GitHub's REST contract verbatim — shithub's S50 sprint
// implements the same envelopes server-side, so this is the same code
// that runs against either host.
package repos

import "time"

// Repo is the canonical repository envelope returned by every read endpoint
// (view, list, search). Field order matches the response shape so JSON
// marshaling round-trips bytewise when needed (golden tests).
type Repo struct {
	ID            int64     `json:"id"`
	NodeID        string    `json:"node_id,omitempty"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Owner         Owner     `json:"owner"`
	Private       bool      `json:"private"`
	Visibility    string    `json:"visibility,omitempty"`
	Description   string    `json:"description,omitempty"`
	Fork          bool      `json:"fork"`
	Archived      bool      `json:"archived"`
	Disabled      bool      `json:"disabled,omitempty"`
	Homepage      string    `json:"homepage,omitempty"`
	Language      string    `json:"language,omitempty"`
	DefaultBranch string    `json:"default_branch"`
	Topics        []string  `json:"topics,omitempty"`
	License       *License  `json:"license,omitempty"`
	Parent        *Repo     `json:"parent,omitempty"`
	Source        *Repo     `json:"source,omitempty"`
	Permissions   *Perms    `json:"permissions,omitempty"`
	Stargazers    int       `json:"stargazers_count"`
	Watchers      int       `json:"watchers_count"`
	Forks         int       `json:"forks_count"`
	OpenIssues    int       `json:"open_issues_count"`
	Size          int       `json:"size"`
	HTMLURL       string    `json:"html_url"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	GitURL        string    `json:"git_url,omitempty"`
	MirrorURL     string    `json:"mirror_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	PushedAt      time.Time `json:"pushed_at,omitempty"`

	// Behavior flags echoed back on edit responses.
	HasIssues           bool `json:"has_issues"`
	HasProjects         bool `json:"has_projects"`
	HasWiki             bool `json:"has_wiki"`
	HasDiscussions      bool `json:"has_discussions,omitempty"`
	AllowForking        bool `json:"allow_forking,omitempty"`
	AllowUpdateBranch   bool `json:"allow_update_branch,omitempty"`
	IsTemplate          bool `json:"is_template,omitempty"`
	DeleteBranchOnMerge bool `json:"delete_branch_on_merge,omitempty"`
}

// Owner is the user-or-org envelope inside a Repo response. Type is "User"
// or "Organization"; both shapes share the same fields on the wire.
type Owner struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Type      string `json:"type,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
}

// License is the SPDX descriptor returned when shithub identifies a
// LICENSE file at HEAD. May be nil even on repos with a license file
// when detection fails.
type License struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	SPDXID string `json:"spdx_id,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Perms reports the authenticated caller's effective access. Mirrors gh:
// `admin` implies the others; `pull` is always true for repos the caller
// can see.
type Perms struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain,omitempty"`
	Push     bool `json:"push"`
	Triage   bool `json:"triage,omitempty"`
	Pull     bool `json:"pull"`
}

// CreateInput is the request body for POST /user/repos and /orgs/{org}/repos.
// Fields are pointers when the zero value is significant (e.g., "private:
// false" must be sent explicitly to create a public repo, since shithub's
// default is private for personal repos).
type CreateInput struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Homepage          string `json:"homepage,omitempty"`
	Private           bool   `json:"private"`
	Visibility        string `json:"visibility,omitempty"`
	HasIssues         *bool  `json:"has_issues,omitempty"`
	HasProjects       *bool  `json:"has_projects,omitempty"`
	HasWiki           *bool  `json:"has_wiki,omitempty"`
	AutoInit          bool   `json:"auto_init,omitempty"`
	GitignoreTemplate string `json:"gitignore_template,omitempty"`
	LicenseTemplate   string `json:"license_template,omitempty"`
	DefaultBranch     string `json:"default_branch,omitempty"`
	IsTemplate        bool   `json:"is_template,omitempty"`
}

// GenerateInput is the body for POST /repos/{owner}/{repo}/generate
// (template instantiation). `Owner` is the destination owner.
type GenerateInput struct {
	Owner              string `json:"owner"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	IncludeAllBranches bool   `json:"include_all_branches,omitempty"`
	Private            bool   `json:"private,omitempty"`
}

// EditInput is the body for PATCH /repos/{owner}/{repo}. Every field is a
// pointer so callers can patch a single attribute without overwriting the
// rest. Marshalling drops nil fields via omitempty.
type EditInput struct {
	Name              *string `json:"name,omitempty"`
	Description       *string `json:"description,omitempty"`
	Homepage          *string `json:"homepage,omitempty"`
	Private           *bool   `json:"private,omitempty"`
	Visibility        *string `json:"visibility,omitempty"`
	HasIssues         *bool   `json:"has_issues,omitempty"`
	HasProjects       *bool   `json:"has_projects,omitempty"`
	HasWiki           *bool   `json:"has_wiki,omitempty"`
	HasDiscussions    *bool   `json:"has_discussions,omitempty"`
	DefaultBranch     *string `json:"default_branch,omitempty"`
	AllowForking      *bool   `json:"allow_forking,omitempty"`
	AllowUpdateBranch *bool   `json:"allow_update_branch,omitempty"`
	Archived          *bool   `json:"archived,omitempty"`
	IsTemplate        *bool   `json:"is_template,omitempty"`
}

// IsEmpty reports whether any patchable field is set. Callers use this
// to decide whether the wire request is worth making at all (vs. only
// topic ops, which go through a separate endpoint).
func (e EditInput) IsEmpty() bool {
	return e.Name == nil && e.Description == nil && e.Homepage == nil &&
		e.Private == nil && e.Visibility == nil &&
		e.HasIssues == nil && e.HasProjects == nil && e.HasWiki == nil &&
		e.HasDiscussions == nil && e.DefaultBranch == nil &&
		e.AllowForking == nil && e.AllowUpdateBranch == nil &&
		e.Archived == nil && e.IsTemplate == nil
}

// ForkInput is the body for POST /repos/{owner}/{repo}/forks. All fields
// optional; empty Organization means "fork to the authenticated user".
type ForkInput struct {
	Organization      string `json:"organization,omitempty"`
	Name              string `json:"name,omitempty"`
	DefaultBranchOnly bool   `json:"default_branch_only,omitempty"`
}

// MergeUpstreamInput is the body for POST /repos/{owner}/{repo}/merge-upstream.
// Branch is the local branch to update; the server figures out the
// upstream from the fork relationship.
type MergeUpstreamInput struct {
	Branch string `json:"branch"`
}

// MergeUpstreamResult is the response body for merge-upstream. Status is
// "fast-forward", "merge", or "non-fast-forward"; the message is suitable
// to surface verbatim.
type MergeUpstreamResult struct {
	Message    string `json:"message"`
	MergeType  string `json:"merge_type,omitempty"`
	BaseBranch string `json:"base_branch,omitempty"`
}

// TopicsPayload is the request/response body for PUT /repos/{owner}/{repo}/topics.
// The field name is plural ("names") to match the gh contract.
type TopicsPayload struct {
	Names []string `json:"names"`
}

// README is the response from GET /repos/{owner}/{repo}/readme. The Content
// is base64-encoded per the gh contract; callers use DecodeContent to get
// the raw bytes.
type README struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int    `json:"size"`
	Encoding    string `json:"encoding"`
	Content     string `json:"content"`
	HTMLURL     string `json:"html_url,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

// ListOptions captures the filter/sort/pagination knobs accepted by the
// repo-list endpoints. Zero values yield the server default behavior.
type ListOptions struct {
	Visibility  string // "public" | "private" | ""
	Affiliation string
	Type        string // "all" | "owner" | "public" | "private" | "member" | "fork" | "source"
	Sort        string // "created" | "updated" | "pushed" | "full_name"
	Direction   string // "asc" | "desc"
	PerPage     int    // server cap usually 100; client clamps if zero
	Limit       int    // soft cap on total items; <= 0 means "no limit beyond MaxPages"
}
