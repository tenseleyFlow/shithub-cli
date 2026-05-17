// SPDX-License-Identifier: AGPL-3.0-or-later

package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client wraps an authenticated api.Client for the secrets/variables
// endpoints.  All methods are scope-aware: repo-level callers pass owner
// + repo, org-level callers pass owner alone.
type Client struct{ api *api.Client }

// NewClient returns a typed secrets client. Nil-safe.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// ListRepoSecrets paginates the repo-scoped secrets list.
func (c *Client) ListRepoSecrets(ctx context.Context, owner, repo string) ([]Secret, error) {
	return c.listSecrets(ctx, "/repos/{owner}/{repo}/actions/secrets?per_page=100",
		api.WithOwner(owner), api.WithRepo(repo))
}

// ListOrgSecrets paginates the org-scoped secrets list.
func (c *Client) ListOrgSecrets(ctx context.Context, org string) ([]Secret, error) {
	return c.listSecrets(ctx, fmt.Sprintf("/orgs/%s/actions/secrets?per_page=100", url.PathEscape(org)))
}

func (c *Client) listSecrets(ctx context.Context, path string, opts ...api.RequestOption) ([]Secret, error) {
	var out []Secret
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path, opts...) {
		if err != nil {
			return nil, err
		}
		var page SecretsResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("secrets: decode: %w", err)
		}
		out = append(out, page.Secrets...)
	}
	return out, nil
}

// GetRepoPublicKey fetches the repo's sealed-box public key. The
// "not found" sentinel (api.ErrNotFound) is the explicit "server has not
// migrated to sealed-box yet — caller may fall back to plaintext" signal.
func (c *Client) GetRepoPublicKey(ctx context.Context, owner, repo string) (*PublicKey, error) {
	return c.getPublicKey(ctx,
		"/repos/{owner}/{repo}/actions/secrets/public-key",
		api.WithOwner(owner), api.WithRepo(repo))
}

// GetOrgPublicKey fetches the org-scoped public key.
func (c *Client) GetOrgPublicKey(ctx context.Context, org string) (*PublicKey, error) {
	path := fmt.Sprintf("/orgs/%s/actions/secrets/public-key", url.PathEscape(org))
	return c.getPublicKey(ctx, path)
}

func (c *Client) getPublicKey(ctx context.Context, path string, opts ...api.RequestOption) (*PublicKey, error) {
	var out PublicKey
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out, opts...); err != nil {
		return nil, err
	}
	return &out, nil
}

// PutRepoSecret writes a repo-scoped secret.
func (c *Client) PutRepoSecret(ctx context.Context, owner, repo, name string, in SetSecretInput) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/secrets/%s", url.PathEscape(name))
	return c.api.REST(ctx, http.MethodPut, path, in, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// PutOrgSecret writes an org-scoped secret.
func (c *Client) PutOrgSecret(ctx context.Context, org, name string, in SetSecretInput) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/orgs/%s/actions/secrets/%s", url.PathEscape(org), url.PathEscape(name))
	return c.api.REST(ctx, http.MethodPut, path, in, nil)
}

// DeleteRepoSecret removes a repo-scoped secret.
func (c *Client) DeleteRepoSecret(ctx context.Context, owner, repo, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/secrets/%s", url.PathEscape(name))
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// DeleteOrgSecret removes an org-scoped secret.
func (c *Client) DeleteOrgSecret(ctx context.Context, org, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/orgs/%s/actions/secrets/%s", url.PathEscape(org), url.PathEscape(name))
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil)
}

// ListRepoVariables paginates the repo-scoped variables list.
func (c *Client) ListRepoVariables(ctx context.Context, owner, repo string) ([]Variable, error) {
	return c.listVariables(ctx, "/repos/{owner}/{repo}/actions/variables?per_page=100",
		api.WithOwner(owner), api.WithRepo(repo))
}

// ListOrgVariables paginates the org-scoped variables list.
func (c *Client) ListOrgVariables(ctx context.Context, org string) ([]Variable, error) {
	return c.listVariables(ctx, fmt.Sprintf("/orgs/%s/actions/variables?per_page=100", url.PathEscape(org)))
}

func (c *Client) listVariables(ctx context.Context, path string, opts ...api.RequestOption) ([]Variable, error) {
	var out []Variable
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path, opts...) {
		if err != nil {
			return nil, err
		}
		var page VariablesResponse
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("variables: decode: %w", err)
		}
		out = append(out, page.Variables...)
	}
	return out, nil
}

// GetRepoVariable fetches a single repo-scoped variable.
func (c *Client) GetRepoVariable(ctx context.Context, owner, repo, name string) (*Variable, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/variables/%s", url.PathEscape(name))
	var out Variable
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOrgVariable fetches a single org-scoped variable.
func (c *Client) GetOrgVariable(ctx context.Context, org, name string) (*Variable, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/orgs/%s/actions/variables/%s", url.PathEscape(org), url.PathEscape(name))
	var out Variable
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateRepoVariable POSTs a new repo-scoped variable. shithub follows
// the gh convention of separate POST-for-create / PATCH-for-update
// shapes — see UpdateRepoVariable.
func (c *Client) CreateRepoVariable(ctx context.Context, owner, repo string, in SetVariableInput) error {
	if err := validateName(in.Name); err != nil {
		return err
	}
	return c.api.REST(ctx, http.MethodPost,
		"/repos/{owner}/{repo}/actions/variables", in, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// UpdateRepoVariable PATCHes an existing repo-scoped variable.
func (c *Client) UpdateRepoVariable(ctx context.Context, owner, repo, name string, in SetVariableInput) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/variables/%s", url.PathEscape(name))
	return c.api.REST(ctx, http.MethodPatch, path, in, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// DeleteRepoVariable removes a repo-scoped variable.
func (c *Client) DeleteRepoVariable(ctx context.Context, owner, repo, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/repos/{owner}/{repo}/actions/variables/%s", url.PathEscape(name))
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// CreateOrgVariable POSTs a new org-scoped variable.
func (c *Client) CreateOrgVariable(ctx context.Context, org string, in SetVariableInput) error {
	if err := validateName(in.Name); err != nil {
		return err
	}
	path := fmt.Sprintf("/orgs/%s/actions/variables", url.PathEscape(org))
	return c.api.REST(ctx, http.MethodPost, path, in, nil)
}

// UpdateOrgVariable PATCHes an existing org-scoped variable.
func (c *Client) UpdateOrgVariable(ctx context.Context, org, name string, in SetVariableInput) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/orgs/%s/actions/variables/%s", url.PathEscape(org), url.PathEscape(name))
	return c.api.REST(ctx, http.MethodPatch, path, in, nil)
}

// DeleteOrgVariable removes an org-scoped variable.
func (c *Client) DeleteOrgVariable(ctx context.Context, org, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	path := fmt.Sprintf("/orgs/%s/actions/variables/%s", url.PathEscape(org), url.PathEscape(name))
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil)
}

// validateName enforces GitHub Actions secret-name rules. Match gh /
// GitHub: ^[A-Z_][A-Z0-9_]*$, can't start with GITHUB_, no
// lowercase (C-audit C25). The pre-D3d implementation accepted
// lowercase silently which then mismatched gh-compat scripts.
func validateName(name string) error {
	if name == "" {
		return errors.New("secrets: name is required")
	}
	if strings.HasPrefix(name, "GITHUB_") {
		return fmt.Errorf("secrets: name %q is reserved (cannot start with GITHUB_)", name)
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
		case r == '_':
		case r >= '0' && r <= '9':
			if i == 0 {
				return fmt.Errorf("secrets: name %q cannot start with a digit", name)
			}
		default:
			return fmt.Errorf("secrets: invalid character %q in name %q (allowed: A-Z, 0-9, _)", r, name)
		}
	}
	return nil
}
