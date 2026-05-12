// SPDX-License-Identifier: AGPL-3.0-or-later

package keys

import (
	"context"
	"fmt"
	"net/http"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// Client wraps an authenticated api.Client for the /user/keys and
// /user/gpg_keys endpoints.
type Client struct{ api *api.Client }

// NewClient returns a typed keys client. Nil-safe.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// ListSSH returns every SSH key registered against the authenticated user.
func (c *Client) ListSSH(ctx context.Context) ([]SSHKey, error) {
	var out []SSHKey
	if err := c.api.REST(ctx, http.MethodGet, "/user/keys", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddSSH uploads a new SSH key. The server returns the canonical
// envelope including the assigned ID and the fingerprint it computed.
func (c *Client) AddSSH(ctx context.Context, in SSHKeyInput) (*SSHKey, error) {
	var out SSHKey
	if err := c.api.REST(ctx, http.MethodPost, "/user/keys", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteSSH removes an SSH key by ID. Server returns 204 on success.
func (c *Client) DeleteSSH(ctx context.Context, id int64) error {
	path := fmt.Sprintf("/user/keys/%d", id)
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil)
}

// ListGPG returns every GPG key registered against the authenticated user.
func (c *Client) ListGPG(ctx context.Context) ([]GPGKey, error) {
	var out []GPGKey
	if err := c.api.REST(ctx, http.MethodGet, "/user/gpg_keys", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddGPG uploads a new GPG key.
func (c *Client) AddGPG(ctx context.Context, in GPGKeyInput) (*GPGKey, error) {
	var out GPGKey
	if err := c.api.REST(ctx, http.MethodPost, "/user/gpg_keys", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteGPG removes a GPG key by ID.
func (c *Client) DeleteGPG(ctx context.Context, id int64) error {
	path := fmt.Sprintf("/user/gpg_keys/%d", id)
	return c.api.REST(ctx, http.MethodDelete, path, nil, nil)
}
