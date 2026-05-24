// SPDX-License-Identifier: AGPL-3.0-or-later

// Package pulls owns the typed wire shape and helper client for shithub's
// /api/v1/repos/{owner}/{repo}/pulls surface. Field tags mirror gh / GitHub
// REST so shithub's S50 §4 implementation can reuse the same envelopes.
package pulls

import (
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
)

// PR is the canonical pull-request envelope. The Issue-shaped fields
// (labels/assignees/milestone/comments) live on the same record because
// shithub mirrors GitHub's "issues are PRs are issues" data model.
type PR struct {
	ID             int64             `json:"id"`
	NodeID         string            `json:"node_id,omitempty"`
	Number         int               `json:"number"`
	Title          string            `json:"title"`
	Body           string            `json:"body,omitempty"`
	State          string            `json:"state"` // "open" | "closed"
	Draft          bool              `json:"draft,omitempty"`
	Merged         bool              `json:"merged,omitempty"`
	Mergeable      *bool             `json:"mergeable,omitempty"`
	MergeableState string            `json:"mergeable_state,omitempty"`
	ReviewDecision string            `json:"review_decision,omitempty"` // server-derived (S50 §4)
	User           *api.User         `json:"user,omitempty"`
	Assignees      []api.User        `json:"assignees,omitempty"`
	RequestedRevs  []api.User        `json:"requested_reviewers,omitempty"`
	Labels         []issues.Label    `json:"labels,omitempty"`
	Milestone      *issues.Milestone `json:"milestone,omitempty"`
	Head           Ref               `json:"head"`
	Base           Ref               `json:"base"`
	Comments       int               `json:"comments"`
	ReviewComments int               `json:"review_comments,omitempty"`
	Commits        int               `json:"commits,omitempty"`
	Additions      int               `json:"additions,omitempty"`
	Deletions      int               `json:"deletions,omitempty"`
	ChangedFiles   int               `json:"changed_files,omitempty"`
	Locked         bool              `json:"locked,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	ClosedAt       *time.Time        `json:"closed_at,omitempty"`
	MergedAt       *time.Time        `json:"merged_at,omitempty"`
	MergedBy       *api.User         `json:"merged_by,omitempty"`
	// AuthorAssociation is gh's 5-value enum (OWNER / MEMBER /
	// COLLABORATOR / CONTRIBUTOR / NONE). Populated by I7a server-side.
	AuthorAssociation string          `json:"author_association,omitempty"`
	HTMLURL           string          `json:"html_url"`
	DiffURL           string          `json:"diff_url,omitempty"`
	PatchURL          string          `json:"patch_url,omitempty"`
	Repository        *issues.RepoRef `json:"repository,omitempty"`

	// Reviews is a server-side convenience: when set, lists the most-recent
	// review per reviewer. Optional; clients still hit /reviews for the
	// full history (C10 territory).
	Reviews []Review `json:"reviews,omitempty"`
}

// IsClosed reports whether the PR is closed (regardless of merged state).
func (p PR) IsClosed() bool { return p.State == "closed" }

// IsMerged reports whether the PR was merged. Distinct from closed-but-
// unmerged ("declined" in some UIs).
func (p PR) IsMerged() bool { return p.Merged }

// Ref describes one side of a PR (head or base). Sha is the commit SHA
// at the time the envelope was emitted; the Ref name is server-stable
// (mutates when the branch moves).
type Ref struct {
	Label string    `json:"label"` // "owner:branch"
	Ref   string    `json:"ref"`   // "branch"
	SHA   string    `json:"sha"`
	User  *api.User `json:"user,omitempty"`
	Repo  *RepoLite `json:"repo,omitempty"`
}

// RepoLite is the minimal repo envelope embedded inside Ref. We keep it
// here (rather than reusing repos.Repo) to avoid the import cycle that
// would otherwise pull repos into pulls into repos.
type RepoLite struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	DefaultBranch string    `json:"default_branch,omitempty"`
	Private       bool      `json:"private,omitempty"`
	Fork          bool      `json:"fork,omitempty"`
	HTMLURL       string    `json:"html_url,omitempty"`
	CloneURL      string    `json:"clone_url,omitempty"`
	SSHURL        string    `json:"ssh_url,omitempty"`
	Owner         *api.User `json:"owner,omitempty"`
}

// Review is one approval/changes-requested entry on a PR. Full review
// management is C10; PR-core only reads it for display.
type Review struct {
	ID          int64     `json:"id"`
	User        *api.User `json:"user,omitempty"`
	Body        string    `json:"body,omitempty"`
	State       string    `json:"state"` // APPROVED | CHANGES_REQUESTED | COMMENTED | DISMISSED | PENDING
	CommitID    string    `json:"commit_id,omitempty"`
	SubmittedAt time.Time `json:"submitted_at,omitempty"`
	HTMLURL     string    `json:"html_url,omitempty"`
}

// File is one entry in /pulls/{n}/files.
type File struct {
	SHA          string `json:"sha,omitempty"`
	Filename     string `json:"filename"`
	Status       string `json:"status"` // added | modified | removed | renamed | copied
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	Changes      int    `json:"changes"`
	Patch        string `json:"patch,omitempty"`
	PreviousName string `json:"previous_filename,omitempty"`
}

// Commit is one entry in /pulls/{n}/commits. The shape is the abridged
// commit envelope shithub returns for PR-context queries.
type Commit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url,omitempty"`
	Commit  struct {
		Message string `json:"message"`
		Author  *struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"author,omitempty"`
	} `json:"commit"`
	Author    *api.User `json:"author,omitempty"`
	Committer *api.User `json:"committer,omitempty"`
}

// CreateInput is the body for POST /pulls.
type CreateInput struct {
	Title             string `json:"title"`
	Body              string `json:"body,omitempty"`
	Head              string `json:"head"` // "owner:branch" or "branch"
	HeadRepo          string `json:"head_repo,omitempty"`
	Base              string `json:"base"` // "branch"
	Draft             bool   `json:"draft,omitempty"`
	MaintainerCanEdit *bool  `json:"maintainer_can_modify,omitempty"`
}

// EditInput is the body for PATCH /pulls/{n}. Pointers for fields where
// the zero value matters (Draft, MaintainerCanEdit). Labels/assignees go
// through their dedicated issue endpoints — only PR-specific scalars and
// base/title/body/state live here.
type EditInput struct {
	Title             *string `json:"title,omitempty"`
	Body              *string `json:"body,omitempty"`
	Base              *string `json:"base,omitempty"`
	State             *string `json:"state,omitempty"`
	Draft             *bool   `json:"draft,omitempty"`
	MaintainerCanEdit *bool   `json:"maintainer_can_modify,omitempty"`
}

// IsEmpty reports whether the patch carries any changes.
func (e EditInput) IsEmpty() bool {
	return e.Title == nil && e.Body == nil && e.Base == nil && e.State == nil &&
		e.Draft == nil && e.MaintainerCanEdit == nil
}

// UpdateBranchInput is the body for PUT /pulls/{n}/update-branch.
type UpdateBranchInput struct {
	// ExpectedHeadSHA, when set, is the commit SHA the caller expects to
	// be at the head of the PR branch. The server uses it to reject the
	// update when a concurrent push has moved the branch (avoids surprise
	// fast-forwards).
	ExpectedHeadSHA string `json:"expected_head_sha,omitempty"`
}

// UpdateBranchResult is the response from update-branch.
type UpdateBranchResult struct {
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
}

// ListOptions captures the filter knobs accepted by GET /pulls. The
// gh-compatible filters are mapped onto shithub's REST contract in
// encodeListQuery; this struct stays UX-shaped so command code reads
// like the cobra flags that drive it.
type ListOptions struct {
	State     string // "open" | "closed" | "merged" | "all"
	Head      string // "user:branch" or "branch"
	Base      string
	Sort      string // "created" | "updated" | "popularity" | "long-running"
	Direction string // "asc" | "desc"
	PerPage   int
	Limit     int
	Author    string
	Assignee  string
	Labels    []string
	Draft     *bool
}
