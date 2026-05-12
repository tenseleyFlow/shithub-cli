// SPDX-License-Identifier: AGPL-3.0-or-later

package orgs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client wraps an authenticated api.Client for the /orgs surface.
type Client struct{ api *api.Client }

// NewClient returns a typed orgs client. Nil-safe.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// ListAuthenticated paginates /user/orgs and returns every org the
// authed user belongs to (subject to opts.Limit).
func (c *Client) ListAuthenticated(ctx context.Context, opts ListOptions) ([]Org, error) {
	return c.paginate(ctx, "/user/orgs", opts)
}

// ListUser paginates /users/{user}/orgs — the public org list for a
// given user. Returns only orgs the user has chosen to surface.
func (c *Client) ListUser(ctx context.Context, user string, opts ListOptions) ([]Org, error) {
	if user == "" {
		return nil, fmt.Errorf("orgs: empty user")
	}
	path := fmt.Sprintf("/users/%s/orgs", url.PathEscape(user))
	return c.paginate(ctx, path, opts)
}

// Get fetches /orgs/{org} — the full profile envelope.
func (c *Client) Get(ctx context.Context, org string) (*Org, error) {
	if org == "" {
		return nil, fmt.Errorf("orgs: empty org")
	}
	path := fmt.Sprintf("/orgs/%s", url.PathEscape(org))
	var out Org
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// paginate is the shared engine for the list endpoints.
func (c *Client) paginate(ctx context.Context, endpoint string, opts ListOptions) ([]Org, error) {
	if q := encodeListQuery(opts); q != "" {
		endpoint += "?" + q
	}
	var out []Org
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, endpoint) {
		if err != nil {
			return nil, err
		}
		var page []Org
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("orgs: decode page: %w", err)
		}
		out = append(out, page...)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			out = out[:opts.Limit]
			break
		}
	}
	return out, nil
}

// encodeListQuery clamps PerPage and renders the query string. Default
// 30 matches gh.
func encodeListQuery(o ListOptions) string {
	v := url.Values{}
	perPage := o.PerPage
	switch {
	case perPage <= 0:
		perPage = 30
	case perPage > 100:
		perPage = 100
	}
	v.Set("per_page", strconv.Itoa(perPage))
	return v.Encode()
}
