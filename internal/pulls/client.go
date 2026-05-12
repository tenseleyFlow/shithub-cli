// SPDX-License-Identifier: AGPL-3.0-or-later

package pulls

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client is the typed shithub /pulls client. Wrap an api.Client with
// NewClient; method names mirror the gh `pr` subcommand surface.
type Client struct{ api *api.Client }

// NewClient wraps an authenticated api.Client. Returns nil when api is nil.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// Create posts a new PR.
func (c *Client) Create(ctx context.Context, owner, repo string, in CreateInput) (*PR, error) {
	var out PR
	if err := c.api.REST(ctx, http.MethodPost, "/repos/{owner}/{repo}/pulls", in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// View fetches a single PR by number.
func (c *Client) View(ctx context.Context, owner, repo string, number int) (*PR, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d", number)
	var out PR
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Edit patches a PR. Pass an EditInput populated only with fields you
// intend to change; nil pointers are omitted via json/encoding.
func (c *Client) Edit(ctx context.Context, owner, repo string, number int, in EditInput) (*PR, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d", number)
	var out PR
	if err := c.api.REST(ctx, http.MethodPatch, path, in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateBranch issues a PUT against /pulls/{n}/update-branch to merge
// (or rebase, if the server supports it via a header) the base into the
// PR's head branch. Server-side merging avoids the local fetch+merge
// dance for the common fast-forward case.
func (c *Client) UpdateBranch(ctx context.Context, owner, repo string, number int, in UpdateBranchInput, rebase bool) (*UpdateBranchResult, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/update-branch", number)
	opts := []api.RequestOption{api.WithOwner(owner), api.WithRepo(repo)}
	if rebase {
		// shithub honors a strategy hint via a header; the server falls
		// back to merge when the hint is missing.
		opts = append(opts, api.WithHeader("X-Shithub-Strategy", "rebase"))
	}
	var out UpdateBranchResult
	if err := c.api.REST(ctx, http.MethodPut, path, in, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListFiles returns the file change set for a PR.
func (c *Client) ListFiles(ctx context.Context, owner, repo string, number int) ([]File, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/files", number)
	var out []File
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page []File
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("pulls: decode files: %w", err)
		}
		out = append(out, page...)
	}
	return out, nil
}

// ListCommits returns the commits behind a PR.
func (c *Client) ListCommits(ctx context.Context, owner, repo string, number int) ([]Commit, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/pulls/%d/commits", number)
	var out []Commit
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page []Commit
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("pulls: decode commits: %w", err)
		}
		out = append(out, page...)
	}
	return out, nil
}

// Diff fetches the raw unified diff for a PR. shithub exposes
// /owner/repo/pulls/{n}.diff outside /api/v1; we compose the absolute
// URL ourselves so the api.Client doesn't prepend /api/v1.
func (c *Client) Diff(ctx context.Context, owner, repo string, number int) ([]byte, error) {
	abs := fmt.Sprintf("%s/%s/%s/pulls/%d.diff",
		c.api.BaseURL(), url.PathEscape(owner), url.PathEscape(repo), number)
	resp, err := c.api.RESTRaw(ctx, http.MethodGet, abs, nil, api.WithAccept("text/plain"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// List paginates /pulls with the filter set encoded onto the query string.
func (c *Client) List(ctx context.Context, owner, repo string, opts ListOptions) ([]PR, error) {
	q := encodeListQuery(opts)
	path := "/repos/{owner}/{repo}/pulls"
	if q != "" {
		path += "?" + q
	}
	var out []PR
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page []PR
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("pulls: decode page: %w", err)
		}
		out = applyClientFilters(out, page, opts)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			return out[:opts.Limit], nil
		}
	}
	return out, nil
}

// applyClientFilters narrows results with filters the /pulls endpoint
// doesn't honor natively. Currently only --draft (server lacks a query
// param) and the gh "state=merged" virtual (REST has open/closed; merged
// is closed+merged=true). Author/assignee/labels are pre-filtered on
// the server when available; we just don't double-filter here.
func applyClientFilters(acc, page []PR, opts ListOptions) []PR {
	for _, p := range page {
		if opts.State == "merged" && !p.Merged {
			continue
		}
		if opts.Draft != nil && p.Draft != *opts.Draft {
			continue
		}
		acc = append(acc, p)
	}
	return acc
}

// encodeListQuery composes the /pulls filter set.
func encodeListQuery(o ListOptions) string {
	v := url.Values{}
	switch o.State {
	case "open", "closed":
		v.Set("state", o.State)
	case "all", "merged":
		v.Set("state", "all")
	}
	if o.Head != "" {
		v.Set("head", o.Head)
	}
	if o.Base != "" {
		v.Set("base", o.Base)
	}
	if o.Sort != "" {
		v.Set("sort", o.Sort)
	}
	if o.Direction != "" {
		v.Set("direction", o.Direction)
	}
	if len(o.Labels) > 0 {
		v.Set("labels", strings.Join(o.Labels, ","))
	}
	if o.Author != "" {
		v.Set("creator", o.Author)
	}
	if o.Assignee != "" {
		v.Set("assignee", o.Assignee)
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
