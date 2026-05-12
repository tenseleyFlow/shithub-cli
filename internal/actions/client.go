// SPDX-License-Identifier: AGPL-3.0-or-later

package actions

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

// Client wraps an authenticated api.Client for /actions endpoints.
type Client struct{ api *api.Client }

// NewClient returns a typed actions client. Nil-safe.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// ListWorkflows returns every workflow registered against owner/repo,
// paginating through the server's response. Empty result is not an
// error; the caller decides whether to render or warn.
func (c *Client) ListWorkflows(ctx context.Context, owner, repo string) ([]Workflow, error) {
	path := "/repos/{owner}/{repo}/actions/workflows?per_page=100"
	var out []Workflow
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page WorkflowsResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("actions: decode workflows: %w", err)
		}
		out = append(out, page.Workflows...)
	}
	return out, nil
}

// GetWorkflow fetches one workflow by numeric ID or file path. The
// server accepts both forms at the same endpoint.
func (c *Client) GetWorkflow(ctx context.Context, owner, repo, ref string) (*Workflow, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/workflows/%s", url.PathEscape(ref))
	var out Workflow
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Dispatch triggers a workflow_dispatch event. shithub S41a validates
// the inputs map against the workflow's declared input schema; the
// server returns 204 on accept and a typed error otherwise.
func (c *Client) Dispatch(ctx context.Context, owner, repo, ref string, in DispatchInput) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/workflows/%s/dispatches", url.PathEscape(ref))
	return c.api.REST(ctx, http.MethodPost, path, in, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// ListRuns paginates /actions/runs with the supplied filters. Limit
// caps the total items returned; when zero, the default 30-page cap
// in the api package applies.
func (c *Client) ListRuns(ctx context.Context, owner, repo string, opts RunListOptions) ([]WorkflowRun, error) {
	endpoint := "/repos/{owner}/{repo}/actions/runs"
	if opts.WorkflowFile != "" {
		// gh exposes a per-workflow shortcut endpoint.
		endpoint = fmt.Sprintf("/repos/{owner}/{repo}/actions/workflows/%s/runs", url.PathEscape(opts.WorkflowFile))
	}
	q := encodeRunQuery(opts)
	if q != "" {
		endpoint += "?" + q
	}

	var out []WorkflowRun
	collected := 0
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, endpoint,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page RunsResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("actions: decode runs: %w", err)
		}
		for _, r := range page.WorkflowRuns {
			out = append(out, r)
			collected++
			if opts.Limit > 0 && collected >= opts.Limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// GetRun fetches a single workflow run.
func (c *Client) GetRun(ctx context.Context, owner, repo string, id int64) (*WorkflowRun, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/runs/%d", id)
	var out WorkflowRun
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListJobs returns the jobs (with steps) for a workflow run.
func (c *Client) ListJobs(ctx context.Context, owner, repo string, runID int64) ([]Job, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/runs/%d/jobs?per_page=100", runID)
	var out []Job
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page JobsResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("actions: decode jobs: %w", err)
		}
		out = append(out, page.Jobs...)
	}
	return out, nil
}

// ListArtifacts returns the artifacts uploaded by a workflow run.
func (c *Client) ListArtifacts(ctx context.Context, owner, repo string, runID int64) ([]Artifact, error) {
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/runs/%d/artifacts?per_page=100", runID)
	var out []Artifact
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page ArtifactsResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("actions: decode artifacts: %w", err)
		}
		out = append(out, page.Artifacts...)
	}
	return out, nil
}

// DownloadArtifact streams the artifact archive (zip) to dst. The
// caller is responsible for closing dst. The server may issue a 302 to
// the object store; the underlying http.Client follows by default.
func (c *Client) DownloadArtifact(ctx context.Context, owner, repo string, artifactID int64, dst io.Writer) error {
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/artifacts/%d/zip", artifactID)
	resp, err := c.api.RESTRaw(ctx, http.MethodGet, path, nil,
		api.WithOwner(owner), api.WithRepo(repo))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(dst, resp.Body); err != nil {
		return fmt.Errorf("actions: artifact download: %w", err)
	}
	return nil
}

// ListCaches paginates /actions/caches with the supplied filters.
func (c *Client) ListCaches(ctx context.Context, owner, repo string, opts CacheListOptions) ([]Cache, error) {
	endpoint := "/repos/{owner}/{repo}/actions/caches"
	q := encodeCacheQuery(opts)
	if q != "" {
		endpoint += "?" + q
	}
	var out []Cache
	collected := 0
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, endpoint,
		api.WithOwner(owner), api.WithRepo(repo)) {
		if err != nil {
			return nil, err
		}
		var page CachesResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("actions: decode caches: %w", err)
		}
		for _, c := range page.ActionsCaches {
			out = append(out, c)
			collected++
			if opts.Limit > 0 && collected >= opts.Limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// encodeRunQuery composes the filter query string for ListRuns.
func encodeRunQuery(o RunListOptions) string {
	v := url.Values{}
	if o.Branch != "" {
		v.Set("branch", o.Branch)
	}
	if o.Event != "" {
		v.Set("event", o.Event)
	}
	if o.Actor != "" {
		v.Set("actor", o.Actor)
	}
	if o.Status != "" {
		v.Set("status", o.Status)
	}
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

// encodeCacheQuery composes the filter query string for ListCaches.
func encodeCacheQuery(o CacheListOptions) string {
	v := url.Values{}
	if o.Key != "" {
		v.Set("key", o.Key)
	}
	if o.Ref != "" {
		v.Set("ref", o.Ref)
	}
	if o.Sort != "" {
		v.Set("sort", o.Sort)
	}
	if o.Order != "" {
		v.Set("direction", strings.ToLower(o.Order))
	}
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
