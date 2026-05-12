// SPDX-License-Identifier: AGPL-3.0-or-later

package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client wraps an authenticated api.Client and exposes one method per
// /search endpoint. Each method composes the query string, paginates
// through Link headers, and returns a fully decoded Response[T].
type Client struct{ api *api.Client }

// NewClient returns a search Client. Nil-safe: returns nil when a is nil
// so callers can no-op without an explicit check.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// Options drives query-string construction for every /search endpoint.
// Sort/Order are forwarded verbatim (server validates); PerPage is
// clamped to [1, 100]; Limit caps the total items returned across pages
// (0 = unbounded, subject to api.DefaultMaxPages).
type Options struct {
	Sort    string
	Order   string
	PerPage int
	Limit   int
}

// Repositories searches /search/repositories. The full-text query string
// is passed through as `q`; the server runs FTS + pg_trgm and applies
// any qualifiers it understands.
func (c *Client) Repositories(ctx context.Context, query string, opts Options) (*Response[RepoItem], error) {
	return paginate[RepoItem](ctx, c, "/search/repositories", query, opts, nil)
}

// Issues searches /search/issues with `type=issue` so PRs are excluded
// server-side. Callers building a unified query may also embed
// `is:issue` in the user-visible query string — both are accepted by
// the server.
func (c *Client) Issues(ctx context.Context, query string, opts Options) (*Response[IssueItem], error) {
	return paginate[IssueItem](ctx, c, "/search/issues", query, opts, url.Values{"type": {"issue"}})
}

// PullRequests searches /search/issues with `type=pr` so issues are
// excluded server-side. PR-specific fields (draft / merged / mergeable
// state) live on PRItem.
func (c *Client) PullRequests(ctx context.Context, query string, opts Options) (*Response[PRItem], error) {
	return paginate[PRItem](ctx, c, "/search/issues", query, opts, url.Values{"type": {"pr"}})
}

// Code searches /search/code. Snippet previews land in
// CodeMatch.TextMatches when the server forwards ts_headline output.
func (c *Client) Code(ctx context.Context, query string, opts Options) (*Response[CodeItem], error) {
	return paginate[CodeItem](ctx, c, "/search/code", query, opts, nil)
}

// Commits searches /search/commits.
func (c *Client) Commits(ctx context.Context, query string, opts Options) (*Response[CommitItem], error) {
	return paginate[CommitItem](ctx, c, "/search/commits", query, opts, nil)
}

// paginate is the generic engine shared by every typed wrapper. It
// composes `?q=...` + Options into the first-page path, then walks Link
// headers (via api.DoPaginated) decoding each page into Response[T] and
// stitching the Items slice together. The first page's TotalCount /
// IncompleteResults are preserved on the merged result — gh exposes the
// same.
func paginate[T any](
	ctx context.Context,
	c *Client,
	endpoint, query string,
	opts Options,
	extra url.Values,
) (*Response[T], error) {
	if c == nil || c.api == nil {
		return nil, errors.New("search: nil client")
	}
	path := endpoint + "?" + encodeQuery(query, opts, extra)

	merged := &Response[T]{}
	first := true
	collected := 0
	reqOpts := paginateOpts(opts)
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path, reqOpts...) {
		if err != nil {
			return nil, err
		}
		var page Response[T]
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("search: decode %s: %w", endpoint, err)
		}
		if first {
			merged.TotalCount = page.TotalCount
			merged.IncompleteResults = page.IncompleteResults
			first = false
		} else if page.IncompleteResults {
			// If any later page reports incomplete_results, propagate
			// that — it's the conservative read of the GitHub contract.
			merged.IncompleteResults = true
		}
		for i := range page.Items {
			merged.Items = append(merged.Items, page.Items[i])
			collected++
			if opts.Limit > 0 && collected >= opts.Limit {
				return merged, nil
			}
		}
	}
	return merged, nil
}

// paginateOpts derives the page-cap to pass to api.DoPaginated from the
// caller's Limit. Without this, the api package's default 30-page cap
// would silently clip large --limit values (e.g., --limit 5000 caps at
// ~3000 items when PerPage=100). When the user has explicitly set
// Limit, we pass WithMaxPages(-1) to disable the page-cap entirely and
// rely on the per-item `collected >= opts.Limit` check in paginate() as
// the authoritative stop. The risk of an unbounded walk on a runaway
// server is contained: Limit is the hard ceiling on items returned,
// and DoPaginated still stops when the server runs out of Link `next`
// headers.
func paginateOpts(opts Options) []api.RequestOption {
	if opts.Limit <= 0 {
		return nil // fall through to api.DefaultMaxPages
	}
	return []api.RequestOption{api.WithMaxPages(-1)}
}

// encodeQuery composes the query string. The user's full-text `query`
// goes into `q` verbatim — no client-side escaping beyond URL encoding
// — so server-side parsers see exactly what the user typed.
func encodeQuery(query string, opts Options, extra url.Values) string {
	v := url.Values{}
	if query != "" {
		v.Set("q", query)
	}
	if opts.Sort != "" {
		v.Set("sort", opts.Sort)
	}
	if opts.Order != "" {
		v.Set("order", opts.Order)
	}
	perPage := opts.PerPage
	switch {
	case perPage <= 0:
		perPage = 30
	case perPage > 100:
		perPage = 100
	}
	v.Set("per_page", strconv.Itoa(perPage))
	for k, vs := range extra {
		for _, val := range vs {
			v.Add(k, val)
		}
	}
	return v.Encode()
}
