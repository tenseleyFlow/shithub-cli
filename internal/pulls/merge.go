// SPDX-License-Identifier: AGPL-3.0-or-later

package pulls

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// MergeMethod is the gh-compatible merge-strategy enum. The wire field
// is "merge_method" with values "merge" | "squash" | "rebase".
type MergeMethod string

// MergeMethod values.
const (
	MergeMerge  MergeMethod = "merge"
	MergeSquash MergeMethod = "squash"
	MergeRebase MergeMethod = "rebase"
)

// MergeInput is the body for PUT /pulls/{n}/merge.
type MergeInput struct {
	CommitTitle   string      `json:"commit_title,omitempty"`
	CommitMessage string      `json:"commit_message,omitempty"`
	SHA           string      `json:"sha,omitempty"` // expected head SHA (gh's --match-head-commit)
	MergeMethod   MergeMethod `json:"merge_method,omitempty"`
}

// MergeResult is the response from a successful merge.
type MergeResult struct {
	SHA     string `json:"sha"`
	Merged  bool   `json:"merged"`
	Message string `json:"message,omitempty"`
}

// Merge calls PUT /pulls/{n}/merge. The --admin override is communicated
// via the X-Shithub-Admin-Override header (shithub-specific contract);
// the server enforces the corresponding scope on the token.
func (c *Client) Merge(ctx context.Context, owner, repo string, number int, in MergeInput, admin bool) (*MergeResult, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/merge", number)
	opts := []api.RequestOption{api.WithOwner(owner), api.WithRepo(repo)}
	if admin {
		opts = append(opts, api.WithHeader("X-Shithub-Admin-Override", "1"))
	}
	var out MergeResult
	if err := c.api.REST(ctx, http.MethodPut, path, in, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// AutoMergeInput is the body for PUT /pulls/{n}/auto-merge.
type AutoMergeInput struct {
	CommitTitle   string      `json:"commit_title,omitempty"`
	CommitMessage string      `json:"commit_message,omitempty"`
	MergeMethod   MergeMethod `json:"merge_method"`
}

// EnableAutoMerge marks the PR for auto-merge: shithub merges as soon as
// branch protection allows. shithub-side worker drives the actual merge.
func (c *Client) EnableAutoMerge(ctx context.Context, owner, repo string, number int, in AutoMergeInput) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/auto-merge", number)
	return c.api.REST(ctx, http.MethodPut, path, in, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// DisableAutoMerge cancels a pending auto-merge.
func (c *Client) DisableAutoMerge(ctx context.Context, owner, repo string, number int) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/auto-merge", number)
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// DeleteBranch removes a ref on the server. Used after merge when
// --delete-branch is set. Matches GitHub's /git/refs/heads/<branch>
// shape; shithub mirrors it.
func (c *Client) DeleteBranch(ctx context.Context, owner, repo, branch string) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/git/refs/heads/%s", url.PathEscape(branch))
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}
