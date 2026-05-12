// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/git"
)

// EnvRepo lets users pin the {owner}/{repo} substitution out of band so
// scripts don't need to pass `-R` on every invocation.
const EnvRepo = "SHITHUB_REPO"

// placeholderSpec captures the recognized template tokens in a path.
// {owner}/{repo}/{branch} are gh-compatible; we keep the set small.
type placeholderSpec struct {
	Owner  string
	Repo   string
	Branch string
}

// substitute returns path with every recognized placeholder replaced by
// the corresponding value, URL-escaped so a hostile owner/repo can't
// traverse out of the /repos/{owner}/{repo}/... namespace. Re-audit
// #157 (2026-05-12) — the typed api client got this escaping in #132,
// but the raw `shithub api` passthrough pre-substitutes placeholders
// here, before composeURL runs; without the escape, a value containing
// `%2f` (a percent-encoded slash) substitutes literally and the
// server's path canonicalization may decode and traverse.
//
// Missing values for placeholders that DO appear in the path return an
// error so users see a clear "no -R or SHITHUB_REPO set" message
// instead of a 404. Walks the placeholder list in a fixed order so the
// error message for a missing value is deterministic across runs (Go's
// map iteration is intentionally randomized).
func (s placeholderSpec) substitute(path string) (string, error) {
	tokens := []struct{ name, value string }{
		{"{owner}", s.Owner},
		{"{repo}", s.Repo},
		{"{branch}", s.Branch},
	}
	for _, t := range tokens {
		if !strings.Contains(path, t.name) {
			continue
		}
		if t.value == "" {
			return "", fmt.Errorf("api: %s placeholder needs -R owner/repo or %s env", t.name, EnvRepo)
		}
		path = strings.ReplaceAll(path, t.name, url.PathEscape(t.value))
	}
	return path, nil
}

// resolvePlaceholders builds a placeholderSpec from the precedence chain:
//
//  1. --repo / -R flag (`owner/repo`).
//  2. SHITHUB_REPO env var.
//  3. The shithub remote of the current working tree (git remote get-url).
//
// `expectedHost` is the active host the api command is targeting; remote
// URLs that don't match are ignored (gracefully) so a repo with both gh
// and shithub remotes doesn't auto-resolve to the wrong place.
func resolvePlaceholders(repoFlag, expectedHost string) (placeholderSpec, error) {
	if repoFlag != "" {
		owner, repo, err := splitRepoSpec(repoFlag)
		if err != nil {
			return placeholderSpec{}, err
		}
		return placeholderSpec{Owner: owner, Repo: repo}, nil
	}
	if env := os.Getenv(EnvRepo); env != "" {
		owner, repo, err := splitRepoSpec(env)
		if err != nil {
			return placeholderSpec{}, err
		}
		return placeholderSpec{Owner: owner, Repo: repo}, nil
	}
	r, err := git.ResolveRemote("", "origin")
	if err != nil {
		// Not a git working tree (or no origin) — leave spec empty.
		// substitute() will surface a clean error iff a placeholder
		// actually appears in the path.
		return placeholderSpec{}, nil //nolint:nilerr // empty spec is the correct "no info" signal
	}
	if expectedHost != "" && r.Host != expectedHost {
		// The current dir's remote points elsewhere; refuse to silently
		// guess. Empty spec means "user must pass -R if the path uses
		// placeholders".
		return placeholderSpec{}, nil
	}
	return placeholderSpec{Owner: r.Owner, Repo: r.Repo}, nil
}

// splitRepoSpec validates and splits "owner/repo".
func splitRepoSpec(spec string) (owner, repo string, err error) {
	parts := strings.Split(spec, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("api: --repo must be 'owner/repo'")
	}
	return parts[0], parts[1], nil
}
