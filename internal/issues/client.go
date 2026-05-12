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

// ListAcrossRepos hits the user-scoped /issues endpoints (assigned,
// authored, mentioned). The filter set is opaque to this call; we pass
// it through verbatim because the server semantics differ from the
// per-repo endpoint and the caller is the right place to know.
func (c *Client) ListAcrossRepos(ctx context.Context, scope string, opts ListOptions) ([]Issue, error) {
	q := encodeListQuery(opts)
	if scope != "" {
		switch scope {
		case "assigned":
			// /issues (the user's assigned)
		case "created":
			q = appendQuery(q, "filter=created")
		case "mentioned":
			q = appendQuery(q, "filter=mentioned")
		}
	}
	path := "/issues"
	if q != "" {
		path += "?" + q
	}
	return c.paginate(ctx, path, opts.Limit)
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
