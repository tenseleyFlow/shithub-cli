// SPDX-License-Identifier: AGPL-3.0-or-later

package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client wraps an api.Client with the checks-specific methods.
type Client struct{ api *api.Client }

// NewClient builds a Client. Returns nil when api is nil.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// ListForRef returns every check run on the given ref (typically a
// commit SHA, but the server also accepts branch names). Paginates
// through the standard envelope; collapses pages into a flat slice.
func (c *Client) ListForRef(ctx context.Context, owner, repo, ref string) ([]CheckRun, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/commits/%s/check-runs", url.PathEscape(ref))
	var out []CheckRun
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page CheckRunsResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("checks: decode page: %w", err)
		}
		out = append(out, page.CheckRuns...)
	}
	return out, nil
}
