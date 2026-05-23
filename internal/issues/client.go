// SPDX-License-Identifier: AGPL-3.0-or-later

package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client is the typed shithub /issues client. Wrap an api.Client with
// NewClient; method names mirror the gh `issue` subcommand surface.
type Client struct{ api *api.Client }

// NewClient wraps an authenticated api.Client. Returns nil when api is
// nil so call sites don't have to guard against the zero-value typo.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// ListMilestones fetches the milestone catalog for a repo. Used by
// `issue create --milestone <title>` (E-audit E19) to resolve a
// human-readable name into the numeric id the server's create endpoint
// expects. Server pagination doesn't apply here — milestones per repo
// are bounded small enough that a single page suffices for resolution.
func (c *Client) ListMilestones(ctx context.Context, owner, repo string) ([]Milestone, error) {
	var out []Milestone
	if err := c.api.REST(ctx, http.MethodGet, "/repos/{owner}/{repo}/milestones?state=all", nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return out, nil
}

// Create posts a new issue under owner/repo. Returns the server's echoed
// envelope (with number/url filled in).
func (c *Client) Create(ctx context.Context, owner, repo string, in CreateInput) (*Issue, error) {
	var out Issue
	if err := c.api.REST(ctx, http.MethodPost, "/repos/{owner}/{repo}/issues", in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// View fetches a single issue by number.
func (c *Client) View(ctx context.Context, owner, repo string, number int) (*Issue, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/%d", number)
	var out Issue
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Edit patches an issue. Pass an EditInput populated only with the fields
// you intend to change; nil pointers are omitted server-side.
func (c *Client) Edit(ctx context.Context, owner, repo string, number int, in EditInput) (*Issue, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/%d", number)
	var out Issue
	if err := c.api.REST(ctx, http.MethodPatch, path, in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes an issue. Server policy enforces admin scope and may
// soft-delete vs hard-delete; surface its decision via the response code.
func (c *Client) Delete(ctx context.Context, owner, repo string, number int) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/%d", number)
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// Lock flips the issue's lock flag on with an optional reason. Empty
// reason is valid and sends no reason field per the gh contract.
func (c *Client) Lock(ctx context.Context, owner, repo string, number int, reason string) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/%d/lock", number)
	body := LockInput{Reason: reason}
	return c.api.REST(ctx, http.MethodPut, path, body, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// Unlock flips the issue's lock flag off.
func (c *Client) Unlock(ctx context.Context, owner, repo string, number int) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/%d/lock", number)
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// AddComment posts a new comment.
func (c *Client) AddComment(ctx context.Context, owner, repo string, number int, body string) (*Comment, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/%d/comments", number)
	var out Comment
	if err := c.api.REST(ctx, http.MethodPost, path, CommentInput{Body: body}, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListComments returns the comment thread on an issue.
func (c *Client) ListComments(ctx context.Context, owner, repo string, number int) ([]Comment, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/%d/comments", number)
	var out []Comment
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page []Comment
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("issues: decode comments: %w", err)
		}
		out = append(out, page...)
	}
	return out, nil
}

// EditComment patches a single comment by its ID (not number).
func (c *Client) EditComment(ctx context.Context, owner, repo string, commentID int64, body string) (*Comment, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/comments/%d", commentID)
	var out Comment
	if err := c.api.REST(ctx, http.MethodPatch, path, CommentInput{Body: body}, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteComment removes a comment by its ID.
func (c *Client) DeleteComment(ctx context.Context, owner, repo string, commentID int64) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/issues/comments/%d", commentID)
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// List paginates GET /repos/{o}/{r}/issues with the filter set encoded
// onto the query string. opts.Limit caps the total returned items.
func (c *Client) List(ctx context.Context, owner, repo string, opts ListOptions) ([]Issue, error) {
	q := encodeListQuery(opts)
	path := "/repos/{owner}/{repo}/issues"
	if q != "" {
		path += "?" + q
	}
	return c.paginate(ctx, path, opts.Limit, api.WithOwner(owner), api.WithRepo(repo))
}

// ListAcrossRepos returns the user-scoped issue set used by
// `pr status` / `issue status` dashboards. F29: pre-fix this called
// the (unimplemented) `/api/v1/issues` endpoint and 404'd on every
// invocation. We translate to `/search/issues` qualifiers — the
// search endpoint already supports `author:@me` / `assignee:@me`
// scoping and is what gh's own dashboards use under the hood. The
// `mentioned` scope is degraded to an empty result for now: shithub
// has no mention index, and there's no FTS-safe qualifier to express
// "issues that contain @<login> in body/comments" yet. The audit
// explicitly endorses this CLI-side path until a dedicated endpoint
// ships.
//
// Result shape is unchanged — IssueItem is a type alias for Issue.
func (c *Client) ListAcrossRepos(ctx context.Context, scope string, opts ListOptions) ([]Issue, error) {
	if scope == "mentioned" {
		// Deferred: shithub doesn't index mentions yet. Returning
		// nil keeps the dashboard renderable; callers print an empty
		// section header instead of erroring.
		return nil, nil
	}
	var qual string
	switch scope {
	case "assigned":
		qual = "assignee:@me"
	case "created":
		qual = "author:@me"
	default:
		// Empty scope = no qualifier; equivalent to listing everything
		// visible to the user. Match the previous endpoint's behavior.
	}
	v := url.Values{}
	v.Set("q", buildSearchQuery(qual, opts))
	v.Set("per_page", strconv.Itoa(perPageOrDefault(opts.PerPage)))
	path := "/search/issues?" + v.Encode()
	return c.paginateSearch(ctx, path, opts.Limit)
}

// buildSearchQuery composes the `q` parameter for /search/issues.
// Combines the scope qualifier (`author:@me` / `assignee:@me`) with
// state and any caller-supplied filters that map cleanly to gh's
// search query language.
func buildSearchQuery(qualifier string, opts ListOptions) string {
	parts := []string{}
	if qualifier != "" {
		parts = append(parts, qualifier)
	}
	if opts.State != "" && opts.State != "all" {
		parts = append(parts, "state:"+opts.State)
	}
	for _, l := range opts.Labels {
		parts = append(parts, "label:"+l)
	}
	return strings.Join(parts, " ")
}

func perPageOrDefault(n int) int {
	if n <= 0 {
		return 100
	}
	if n > 100 {
		return 100
	}
	return n
}

// paginateSearch walks /search/issues pages, extracting the items
// array per response. The endpoint returns the gh-canonical envelope
// `{total_count, incomplete_results, items: [...]}`, so we decode that
// shape and accumulate items.
func (c *Client) paginateSearch(ctx context.Context, path string, limit int) ([]Issue, error) {
	var out []Issue
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path) {
		if err != nil {
			return nil, err
		}
		var page struct {
			Items []Issue `json:"items"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("issues: decode search page: %w", err)
		}
		out = append(out, page.Items...)
		if limit > 0 && len(out) >= limit {
			return out[:limit], nil
		}
	}
	return out, nil
}

func (c *Client) paginate(ctx context.Context, path string, limit int, opts ...api.RequestOption) ([]Issue, error) {
	var out []Issue
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path, opts...) {
		if err != nil {
			return nil, err
		}
		var page []Issue
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("issues: decode page: %w", err)
		}
		out = append(out, page...)
		if limit > 0 && len(out) >= limit {
			return out[:limit], nil
		}
	}
	return out, nil
}

// encodeListQuery composes the filter set as a URL query string. Empty
// fields are skipped so the server applies its own defaults.
func encodeListQuery(o ListOptions) string {
	v := url.Values{}
	if o.State != "" {
		v.Set("state", o.State)
	}
	if len(o.Labels) > 0 {
		v.Set("labels", strings.Join(o.Labels, ","))
	}
	if o.Sort != "" {
		v.Set("sort", o.Sort)
	}
	if o.Direction != "" {
		v.Set("direction", o.Direction)
	}
	if o.Since != "" {
		v.Set("since", o.Since)
	}
	if o.Author != "" {
		v.Set("creator", o.Author)
	}
	if o.Assignee != "" {
		v.Set("assignee", o.Assignee)
	}
	if o.Mentioned != "" {
		v.Set("mentioned", o.Mentioned)
	}
	if o.Milestone != "" {
		v.Set("milestone", o.Milestone)
	}
	perPage := o.PerPage
	if perPage <= 0 {
		perPage = 100
	}
	if perPage > 100 {
		perPage = 100
	}
	v.Set("per_page", strconv.Itoa(perPage))
	return v.Encode()
}

func appendQuery(existing, kv string) string {
	if existing == "" {
		return kv
	}
	return existing + "&" + kv
}
