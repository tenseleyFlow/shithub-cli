// SPDX-License-Identifier: AGPL-3.0-or-later

// Package shared holds the helpers reused across `shithub issue`
// subcommands: issue argument parsing, label/assignee list normalization,
// and `@me` expansion.
package shared

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// MeToken is the sentinel users pass on `--assignee`/`--mention`/etc. to
// refer to the authenticated user. Centralized so the spelling stays
// consistent across subcommands.
const MeToken = "@me"

// IssueRef names a single issue. Repo carries the owner/name pair so
// commands acting on multiple repos (e.g., `issue status`) keep the
// context attached to each result.
type IssueRef struct {
	Repo   repocmdshared.RepoRef
	Number int
}

// ParseIssueArg accepts "<number>" or a full shithub URL like
// "https://host/owner/repo/issues/123". Returns the parsed ref plus an
// indicator of whether the URL form was used (URL form is authoritative
// for owner/repo; numeric form requires the caller's resolver to fill
// the repo).
func ParseIssueArg(s string, fallback repocmdshared.RepoRef) (IssueRef, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return IssueRef{}, false, errors.New("issue: argument is empty")
	}

	// Strip a leading '#' so users can paste "#123" verbatim.
	s = strings.TrimPrefix(s, "#")

	if strings.Contains(s, "://") {
		ref, n, err := parseIssueURL(s)
		if err != nil {
			return IssueRef{}, false, err
		}
		return IssueRef{Repo: ref, Number: n}, true, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return IssueRef{}, false, fmt.Errorf("issue: expected positive number or URL, got %q", s)
	}
	if fallback.Owner == "" || fallback.Name == "" {
		return IssueRef{}, false, fmt.Errorf("issue: pass -R or run inside a shithub clone (got %q)", s)
	}
	return IssueRef{Repo: fallback, Number: n}, false, nil
}

// parseIssueURL extracts (host, owner, name, number) from a shithub
// issue URL. Accepts both "/issues/NNN" and "/pull/NNN" since the
// /issues endpoint serves both classes; the caller decides whether a
// PR-shaped result is acceptable.
func parseIssueURL(raw string) (repocmdshared.RepoRef, int, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("issue: parse URL %q: %w", raw, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("issue: URL path too short: %q", u.Path)
	}
	switch parts[len(parts)-2] {
	case "issues", "pull":
	default:
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("issue: URL is not an /issues/ or /pull/ link: %q", u.Path)
	}
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || n <= 0 {
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("issue: URL trailing segment not a number: %q", u.Path)
	}
	return repocmdshared.RepoRef{
		Host:  u.Hostname(),
		Owner: parts[len(parts)-4],
		Name:  parts[len(parts)-3],
	}, n, nil
}

// SplitList parses gh-style repeatable / CSV flag values into a clean
// slice. Whitespace is trimmed, empty fragments are dropped, and the
// order from the input is preserved.
func SplitList(vs []string) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		for _, p := range strings.Split(v, ",") {
			t := strings.TrimSpace(p)
			if t == "" {
				continue
			}
			out = append(out, t)
		}
	}
	return out
}

// ExpandMe replaces any MeToken in the list with the authenticated user's
// login. Returns the list verbatim when no expansion is needed (so the
// caller doesn't pay the /user round-trip). Hits the wire at most once
// per Client thanks to api.Client.CurrentUser's cache.
func ExpandMe(ctx context.Context, client *api.Client, list []string) ([]string, error) {
	needs := false
	for _, v := range list {
		if strings.EqualFold(v, MeToken) {
			needs = true
			break
		}
	}
	if !needs {
		return list, nil
	}
	u, err := client.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(list))
	for i, v := range list {
		if strings.EqualFold(v, MeToken) {
			out[i] = u.Login
			continue
		}
		out[i] = v
	}
	return out, nil
}

// ExpandMeSingle expands a single string. Returns the original value when
// it isn't @me. Convenience over ExpandMe for `--author @me` style flags.
func ExpandMeSingle(ctx context.Context, client *api.Client, v string) (string, error) {
	if !strings.EqualFold(v, MeToken) {
		return v, nil
	}
	u, err := client.CurrentUser(ctx)
	if err != nil {
		return "", err
	}
	return u.Login, nil
}

// IssueWebURL composes the browser-visible URL for an issue. Used by
// `--web` flags everywhere in the issue tree.
func IssueWebURL(ref IssueRef) string {
	return fmt.Sprintf("%s/issues/%d", repocmdshared.WebURL(ref.Repo), ref.Number)
}

// NewIssueWebURL composes the URL for "/issues/new" with optional
// prefilled `title` and `body` query params. Used by `issue create --web`.
func NewIssueWebURL(repo repocmdshared.RepoRef, title, body string) string {
	base := fmt.Sprintf("%s/issues/new", repocmdshared.WebURL(repo))
	q := url.Values{}
	if title != "" {
		q.Set("title", title)
	}
	if body != "" {
		q.Set("body", body)
	}
	if encoded := q.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}
