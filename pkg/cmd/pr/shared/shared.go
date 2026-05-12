// SPDX-License-Identifier: AGPL-3.0-or-later

// Package shared holds the helpers reused across `shithub pr`
// subcommands: PR argument parsing (number / URL / branch), the
// branch-to-PR lookup that powers `pr view`/`pr edit` with no arg,
// and the --fill commit-summary helper.
package shared

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// PRRef names a single PR. Like issues.IssueRef but with PR semantics.
type PRRef struct {
	Repo   repocmdshared.RepoRef
	Number int
}

// ParsePRArg accepts:
//
//   - "<number>" or "#<number>" (resolved against the fallback repo)
//   - a full shithub URL "https://host/owner/repo/pull/123"
//   - a branch name (resolved via branch-to-PR lookup)
//
// The branch case requires a non-nil pulls.Client and a non-empty
// fallback repo; that's why the function takes a context — it may hit
// the API to disambiguate.
func ParsePRArg(ctx context.Context, c *pulls.Client, s string, fallback repocmdshared.RepoRef) (PRRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PRRef{}, errors.New("pr: argument is empty")
	}
	s = strings.TrimPrefix(s, "#")

	if strings.Contains(s, "://") {
		ref, n, err := parsePRURL(s)
		if err != nil {
			return PRRef{}, err
		}
		return PRRef{Repo: ref, Number: n}, nil
	}

	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		if fallback.Owner == "" || fallback.Name == "" {
			return PRRef{}, fmt.Errorf("pr: pass -R or run inside a shithub clone (got %q)", s)
		}
		return PRRef{Repo: fallback, Number: n}, nil
	}

	// Treat as a branch name and look up via branch-to-PR.
	if c == nil {
		return PRRef{}, fmt.Errorf("pr: %q is not a number or URL and no client available for branch lookup", s)
	}
	if fallback.Owner == "" || fallback.Name == "" {
		return PRRef{}, fmt.Errorf("pr: branch lookup needs a repo context for %q", s)
	}
	pr, err := FindPRByBranch(ctx, c, fallback, s)
	if err != nil {
		return PRRef{}, err
	}
	return PRRef{Repo: fallback, Number: pr.Number}, nil
}

// parsePRURL extracts (host, owner, name, number) from a shithub PR URL.
// Accepts both /pull/N and /pulls/N tails (gh writes /pull, shithub web
// writes /pulls; the API uses /pulls — we accept either to avoid surprise).
func parsePRURL(raw string) (repocmdshared.RepoRef, int, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("pr: parse URL %q: %w", raw, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 {
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("pr: URL path too short: %q", u.Path)
	}
	switch parts[len(parts)-2] {
	case "pull", "pulls":
	default:
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("pr: URL is not a /pull/ or /pulls/ link: %q", u.Path)
	}
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || n <= 0 {
		return repocmdshared.RepoRef{}, 0, fmt.Errorf("pr: URL trailing segment not a number: %q", u.Path)
	}
	return repocmdshared.RepoRef{
		Host:  u.Hostname(),
		Owner: parts[len(parts)-4],
		Name:  parts[len(parts)-3],
	}, n, nil
}

// FindPRByBranch returns the (open by default) PR whose head ref matches
// the given branch name. Searches in the same repo first; falls back to
// the same-named branch on any fork if no match. Returns NotFound when
// no PR exists for the branch.
func FindPRByBranch(ctx context.Context, c *pulls.Client, repo repocmdshared.RepoRef, branch string) (*pulls.PR, error) {
	// Filter by head=<owner>:<branch>; the server treats the prefix as
	// optional. We pass the bare branch first to keep the wire small;
	// a second pass with the user prefix runs only if the first misses.
	list, err := c.List(ctx, repo.Owner, repo.Name, pulls.ListOptions{
		State: "open",
		Head:  branch,
		Limit: 50,
	})
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if strings.EqualFold(p.Head.Ref, branch) {
			return &p, nil
		}
	}
	return nil, NotFoundError{Repo: repo, Branch: branch}
}

// NotFoundError signals "no PR matches the branch". Carries the repo +
// branch so the CLI layer can render a precise message.
type NotFoundError struct {
	Repo   repocmdshared.RepoRef
	Branch string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("pr: no open PR for branch %s in %s", e.Branch, e.Repo.FullName())
}

// CurrentBranchFromGit reads the working tree's current branch. Empty
// string when not in a repo (caller decides whether that's fatal).
func CurrentBranchFromGit(r git.Runner, dir string) string {
	if r == nil {
		return ""
	}
	b, err := git.CurrentBranch(r, dir)
	if err != nil {
		return ""
	}
	return b
}

// PRWebURL composes the browser-visible PR URL.
func PRWebURL(ref PRRef) string {
	return fmt.Sprintf("%s/pull/%d", repocmdshared.WebURL(ref.Repo), ref.Number)
}

// NewPRWebURL composes the URL for "/compare/<base>...<head>" with
// optional prefilled title/body/draft. Used by `pr create --web`.
func NewPRWebURL(repo repocmdshared.RepoRef, base, head, title, body string, draft bool) string {
	compare := fmt.Sprintf("%s/compare/%s...%s", repocmdshared.WebURL(repo),
		url.PathEscape(base), url.PathEscape(head))
	q := url.Values{}
	if title != "" {
		q.Set("title", title)
	}
	if body != "" {
		q.Set("body", body)
	}
	if draft {
		q.Set("draft", "1")
	}
	if encoded := q.Encode(); encoded != "" {
		return compare + "?expand=1&" + encoded
	}
	return compare + "?expand=1"
}
