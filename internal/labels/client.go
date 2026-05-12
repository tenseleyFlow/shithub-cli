// SPDX-License-Identifier: AGPL-3.0-or-later

package labels

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client is the typed shithub /labels client.
type Client struct{ api *api.Client }

// NewClient wraps an authenticated api.Client. Returns nil when api is nil.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// List paginates over /labels and returns the flat result. The shithub
// endpoint doesn't honor sort/direction natively; we apply them
// client-side after collecting the full set so users get the gh
// experience without a server prereq.
func (c *Client) List(ctx context.Context, owner, repo string, opts ListOptions) ([]Label, error) {
	path := "/repos/{owner}/{repo}/labels"
	if q := encodeListQuery(opts); q != "" {
		path += "?" + q
	}
	var out []Label
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page []Label
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("labels: decode page: %w", err)
		}
		out = append(out, page...)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			out = out[:opts.Limit]
			break
		}
	}
	applySort(out, opts.Sort, opts.Direction)
	return out, nil
}

// Get fetches a single label by name. Server returns 404 with the
// notFound typed error when missing.
func (c *Client) Get(ctx context.Context, owner, repo, name string) (*Label, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/labels/%s", url.PathEscape(name))
	var out Label
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create posts a new label.
func (c *Client) Create(ctx context.Context, owner, repo string, in CreateInput) (*Label, error) {
	var out Label
	if err := c.api.REST(ctx, http.MethodPost, "/repos/{owner}/{repo}/labels", in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Edit patches a label. NewName drives the rename path; nil fields are
// omitted server-side.
func (c *Client) Edit(ctx context.Context, owner, repo, name string, in EditInput) (*Label, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/labels/%s", url.PathEscape(name))
	var out Label
	if err := c.api.REST(ctx, http.MethodPatch, path, in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a label.
func (c *Client) Delete(ctx context.Context, owner, repo, name string) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/labels/%s", url.PathEscape(name))
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// encodeListQuery composes the per_page hint. shithub doesn't expose
// sort on this endpoint, so we collect everything (within Limit) and
// sort client-side in applySort.
func encodeListQuery(o ListOptions) string {
	v := url.Values{}
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

// applySort orders the result in-place by the user's choice. Empty
// sort defaults to "name". Direction defaults to "asc".
func applySort(list []Label, sortField, direction string) {
	if sortField == "" {
		sortField = "name"
	}
	desc := strings.EqualFold(direction, "desc")
	switch sortField {
	case "created":
		sort.SliceStable(list, func(i, j int) bool {
			if desc {
				return list[i].CreatedAt.After(list[j].CreatedAt)
			}
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		})
	default:
		sort.SliceStable(list, func(i, j int) bool {
			if desc {
				return list[i].Name > list[j].Name
			}
			return list[i].Name < list[j].Name
		})
	}
}
