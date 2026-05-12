// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// User is the minimal envelope the CLI uses for `@me` expansion and the
// authenticated-user display name. Fields mirror gh's gh.User wire shape
// (Login is the canonical handle field); we leave unused fields off to
// keep the contract surface small.
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url,omitempty"`
	Type      string `json:"type,omitempty"`
}

// UnmarshalJSON accepts both `login` (gh-canonical) and `username` (the
// field shithub server currently emits) so the CLI works across the
// transition. Once shithub S50 §1 emits `login`, the fallback becomes
// dead code but is harmless; we leave it for one full release cycle of
// migration safety.
func (u *User) UnmarshalJSON(data []byte) error {
	type alias User // avoid infinite recursion
	var aux struct {
		alias
		Username string `json:"username,omitempty"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*u = User(aux.alias)
	if u.Login == "" && aux.Username != "" {
		u.Login = aux.Username
	}
	return nil
}

// whoami caches the /user lookup so a single command invocation that
// expands `@me` in multiple places (e.g., --assignee @me --mention @me)
// only hits the wire once. Mutex protects the cache because callers may
// fan out across goroutines for unrelated work.
type whoamiCache struct {
	mu   sync.Mutex
	user *User
	err  error
	done bool
}

// CurrentUser returns the authenticated user envelope. First call hits
// /api/v1/user; subsequent calls return the cached result. A non-nil
// error is also cached so transient failures don't multiply into N retries
// across `@me` expansion sites — callers retry by spinning up a fresh client.
func (c *Client) CurrentUser(ctx context.Context) (*User, error) {
	c.whoami.mu.Lock()
	defer c.whoami.mu.Unlock()
	if c.whoami.done {
		return c.whoami.user, c.whoami.err
	}
	var u User
	err := c.REST(ctx, http.MethodGet, "/user", nil, &u)
	c.whoami.done = true
	if err != nil {
		c.whoami.err = fmt.Errorf("api: current user: %w", err)
		return nil, c.whoami.err
	}
	c.whoami.user = &u
	return &u, nil
}
