// SPDX-License-Identifier: AGPL-3.0-or-later

package repos

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
)

// HTTP is the minimal slice of *api.Client this package needs. Declared
// as an interface so tests can stub without the fakeapi roundtrip when
// they want to assert on individual call shapes.
type HTTP interface {
	REST(ctx context.Context, method, path string, body, into any, opts ...api.RequestOption) error
	RESTRaw(ctx context.Context, method, path string, body any, opts ...api.RequestOption) (*http.Response, error)
	DoPaginated(ctx context.Context, method, path string, opts ...api.RequestOption) func(yield func(json.RawMessage, error) bool)
}

// Client is the typed shithub /repos client. Wrap an *api.Client with
// NewClient; method names mirror the gh repo subcommand surface so call
// sites read like the cobra layer that drives them.
type Client struct {
	api *api.Client
}

// NewClient wraps an authenticated api.Client. Returns nil if api is nil
// so callers don't have to defend against zero-value typos.
func NewClient(a *api.Client) *Client {
	if a == nil {
		return nil
	}
	return &Client{api: a}
}

// View fetches a single repo by owner/name.
func (c *Client) View(ctx context.Context, owner, repo string) (*Repo, error) {
	var out Repo
	if err := c.api.REST(ctx, http.MethodGet, "/repos/{owner}/{repo}", nil, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAuthenticated returns repos visible to the bearer token. The opts'
// PerPage/Sort/Direction/Visibility/Type/Affiliation are wired to query
// params; Limit caps the total items returned (0 = no soft cap).
func (c *Client) ListAuthenticated(ctx context.Context, opts ListOptions) ([]Repo, error) {
	return c.listPaginated(ctx, "/user/repos", opts)
}

// ListUser returns the public repos of a user (or all repos when the
// authenticated caller is that user).
func (c *Client) ListUser(ctx context.Context, user string, opts ListOptions) ([]Repo, error) {
	return c.listPaginated(ctx, "/users/"+url.PathEscape(user)+"/repos", opts)
}

// ListOrg returns repos belonging to an org. The caller's permissions
// determine whether private repos appear.
func (c *Client) ListOrg(ctx context.Context, org string, opts ListOptions) ([]Repo, error) {
	return c.listPaginated(ctx, "/orgs/"+url.PathEscape(org)+"/repos", opts)
}

// CreateUser creates a personal repo under the authenticated user. Use
// CreateOrg when the destination is an organization.
func (c *Client) CreateUser(ctx context.Context, in CreateInput) (*Repo, error) {
	var out Repo
	if err := c.api.REST(ctx, http.MethodPost, "/user/repos", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateOrg creates a repo under the named organization.
func (c *Client) CreateOrg(ctx context.Context, org string, in CreateInput) (*Repo, error) {
	var out Repo
	path := "/orgs/" + url.PathEscape(org) + "/repos"
	if err := c.api.REST(ctx, http.MethodPost, path, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Generate instantiates a repo from a template repo. The template must
// have is_template=true server-side.
func (c *Client) Generate(ctx context.Context, templateOwner, templateRepo string, in GenerateInput) (*Repo, error) {
	var out Repo
	path := fmt.Sprintf("/repos/%s/%s/generate", url.PathEscape(templateOwner), url.PathEscape(templateRepo))
	if err := c.api.REST(ctx, http.MethodPost, path, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Edit patches a repo. Pass an EditInput with only the fields you intend
// to change; nil fields are omitted by json/encoding so untouched
// attributes survive untouched server-side.
func (c *Client) Edit(ctx context.Context, owner, repo string, in EditInput) (*Repo, error) {
	var out Repo
	if err := c.api.REST(ctx, http.MethodPatch, "/repos/{owner}/{repo}", in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// Rename is a convenience wrapper over Edit that targets the Name field.
// The server is responsible for updating refs/redirects.
func (c *Client) Rename(ctx context.Context, owner, repo, newName string) (*Repo, error) {
	return c.Edit(ctx, owner, repo, EditInput{Name: &newName})
}

// Archive flips the archived flag on.
func (c *Client) Archive(ctx context.Context, owner, repo string) error {
	yes := true
	_, err := c.Edit(ctx, owner, repo, EditInput{Archived: &yes})
	return err
}

// Unarchive flips the archived flag off.
func (c *Client) Unarchive(ctx context.Context, owner, repo string) error {
	no := false
	_, err := c.Edit(ctx, owner, repo, EditInput{Archived: &no})
	return err
}

// Delete permanently removes the repo. Server enforces scope (repo:admin)
// and may issue a confirmation challenge that surfaces as 4xx with a
// message — the CLI layer already prompts the user before calling this.
func (c *Client) Delete(ctx context.Context, owner, repo string) error {
	return c.api.REST(ctx, http.MethodDelete, "/repos/{owner}/{repo}", nil, nil,
		api.WithOwner(owner), api.WithRepo(repo))
}

// Fork creates a fork. Empty input forks under the authenticated user;
// set Organization to fork into an org.
func (c *Client) Fork(ctx context.Context, owner, repo string, in ForkInput) (*Repo, error) {
	var out Repo
	if err := c.api.REST(ctx, http.MethodPost, "/repos/{owner}/{repo}/forks", in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// MergeUpstream issues a server-side fast-forward of a fork's branch from
// its upstream parent. Branch is the local fork's branch to update.
func (c *Client) MergeUpstream(ctx context.Context, owner, repo string, in MergeUpstreamInput) (*MergeUpstreamResult, error) {
	var out MergeUpstreamResult
	if err := c.api.REST(ctx, http.MethodPost, "/repos/{owner}/{repo}/merge-upstream", in, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReplaceTopics overwrites the topic list. Use AddTopic/RemoveTopic when
// the desired mutation is additive rather than wholesale.
func (c *Client) ReplaceTopics(ctx context.Context, owner, repo string, names []string) ([]string, error) {
	if names == nil {
		names = []string{} // server expects an empty array, not null
	}
	var out TopicsPayload
	if err := c.api.REST(ctx, http.MethodPut, "/repos/{owner}/{repo}/topics", TopicsPayload{Names: names}, &out,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, err
	}
	return out.Names, nil
}

// ListTopics returns the current topic list. The server has no
// dedicated GET /topics endpoint — that returned 405 and broke
// `repo edit --add-topic` / `--remove-topic` end-to-end (E-audit E8).
// Topics already ride the repo view payload, so we read them from
// there. Returns a non-nil empty slice when the repo has none.
func (c *Client) ListTopics(ctx context.Context, owner, repo string) ([]string, error) {
	r, err := c.View(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	if r.Topics == nil {
		return []string{}, nil
	}
	return r.Topics, nil
}

// ReadREADME fetches the README envelope (and decoded content). Pass ref=""
// for the default branch. Returns a typed NotFoundError when the repo
// has no README.
func (c *Client) ReadREADME(ctx context.Context, owner, repo, ref string) (*README, []byte, error) {
	path := "/repos/{owner}/{repo}/readme"
	if ref != "" {
		path += "?ref=" + url.QueryEscape(ref)
	}
	var meta README
	if err := c.api.REST(ctx, http.MethodGet, path, nil, &meta,
		api.WithOwner(owner), api.WithRepo(repo)); err != nil {
		return nil, nil, err
	}
	raw, err := decodeContent(meta.Encoding, meta.Content)
	if err != nil {
		return &meta, nil, err
	}
	return &meta, raw, nil
}

// listPaginated walks DoPaginated, decoding each page as []Repo and
// honoring opts.Limit as a soft cap on total returned items.
func (c *Client) listPaginated(ctx context.Context, path string, opts ListOptions) ([]Repo, error) {
	q := encodeListQuery(opts)
	if q != "" {
		path += "?" + q
	}
	var out []Repo
	for raw, err := range c.api.DoPaginated(ctx, http.MethodGet, path) {
		if err != nil {
			return nil, err
		}
		var page []Repo
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("repos: decode page: %w", err)
		}
		out = append(out, page...)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			out = out[:opts.Limit]
			return out, nil
		}
	}
	return out, nil
}

// encodeListQuery composes the URL query string for the list endpoints.
// Empty fields are skipped so the server sees its own defaults.
func encodeListQuery(o ListOptions) string {
	v := url.Values{}
	if o.Visibility != "" {
		v.Set("visibility", o.Visibility)
	}
	if o.Affiliation != "" {
		v.Set("affiliation", o.Affiliation)
	}
	if o.Type != "" {
		v.Set("type", o.Type)
	}
	if o.Sort != "" {
		v.Set("sort", o.Sort)
	}
	if o.Direction != "" {
		v.Set("direction", o.Direction)
	}
	perPage := o.PerPage
	if perPage <= 0 {
		perPage = 100
	}
	if perPage > 100 {
		perPage = 100
	}
	v.Set("per_page", fmt.Sprintf("%d", perPage))
	return v.Encode()
}

// decodeContent decodes the base64-or-plain content field from a README
// envelope. shithub's contract is base64 like gh's; the branch on
// encoding=="" keeps us forward-compatible if shithub adds a raw mode.
func decodeContent(encoding, content string) ([]byte, error) {
	switch encoding {
	case "", "utf-8", "utf8":
		return []byte(content), nil
	case "base64":
		// strip newlines that some encoders insert.
		clean := strings.ReplaceAll(content, "\n", "")
		return base64.StdEncoding.DecodeString(clean)
	default:
		return nil, fmt.Errorf("repos: unsupported readme encoding %q", encoding)
	}
}
