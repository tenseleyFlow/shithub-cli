// SPDX-License-Identifier: AGPL-3.0-or-later

// Package git holds the thin shell-out wrappers around the canonical git
// binary. Today the surface is just `remote get-url` parsing for -R
// auto-resolution; later sprints (C07 repo) extend it for clone, fetch,
// push wiring.
package git

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	"github.com/cli/safeexec"
)

// Remote captures the parsed owner/repo of a git remote pointing at a
// shithub host. We do not preserve the full URL because callers don't
// need it — placeholder substitution wants strings.
type Remote struct {
	Host  string // bare host, lowercased, no scheme
	Owner string
	Repo  string
}

// String renders the canonical "owner/repo" identifier.
func (r Remote) String() string {
	if r.Owner == "" || r.Repo == "" {
		return ""
	}
	return r.Owner + "/" + r.Repo
}

// ResolveRemote runs `git -C dir remote get-url <name>` and parses the
// result. Empty dir means cwd. When the remote exists but is not a
// shithub URL (e.g., user has both gh and shithub remotes), the parse
// succeeds and the host is carried through — callers compare against
// their configured shithub host to decide whether to accept the match.
//
// Returns an error if git is not on PATH, the dir is not a git working
// tree, or the remote is not configured.
func ResolveRemote(dir, name string) (Remote, error) {
	if name == "" {
		name = "origin"
	}
	bin, err := safeexec.LookPath("git")
	if err != nil {
		return Remote{}, fmt.Errorf("git: locate binary: %w", err)
	}
	args := []string{"remote", "get-url", name}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // bin from safeexec; args from trusted callers
	out, err := cmd.Output()
	if err != nil {
		// Don't leak git's stderr verbatim — wrap so callers get a
		// stable surface. The most common cause is "not a git working
		// tree" or "no remote configured".
		return Remote{}, fmt.Errorf("git remote get-url %s: %w", name, err)
	}
	return ParseRemoteURL(strings.TrimSpace(string(out)))
}

// ParseRemoteURL extracts (host, owner, repo) from any of the URL shapes
// git emits. Accepts:
//
//   - https://host/owner/repo[.git]
//   - http://host/owner/repo[.git]
//   - git@host:owner/repo[.git]
//   - ssh://[user@]host[:port]/owner/repo[.git]
//
// Returns an error on any other shape; we do NOT silently fall back so
// upstream callers can show the user exactly which URL they need to
// reformat.
func ParseRemoteURL(raw string) (Remote, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Remote{}, errors.New("git: empty remote URL")
	}

	// SCP-like form: git@host:owner/repo
	if !strings.Contains(raw, "://") && strings.Contains(raw, "@") && strings.Contains(raw, ":") {
		at := strings.IndexByte(raw, '@')
		colon := strings.IndexByte(raw, ':')
		if at >= 0 && colon > at {
			host := raw[at+1 : colon]
			ownerRepo := raw[colon+1:]
			return splitOwnerRepo(host, ownerRepo)
		}
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Remote{}, fmt.Errorf("git: parse %q: %w", raw, err)
	}
	switch u.Scheme {
	case "https", "http", "ssh", "git":
	default:
		return Remote{}, fmt.Errorf("git: unsupported scheme %q in %q", u.Scheme, raw)
	}
	host := u.Hostname()
	return splitOwnerRepo(host, strings.TrimPrefix(u.Path, "/"))
}

// splitOwnerRepo accepts a "owner/repo[.git]" tail and returns the
// normalized Remote. Returns an error if the path has fewer or more
// segments than expected.
func splitOwnerRepo(host, tail string) (Remote, error) {
	tail = strings.TrimSuffix(tail, ".git")
	parts := strings.Split(tail, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Remote{}, fmt.Errorf("git: expected owner/repo path, got %q", tail)
	}
	return Remote{
		Host:  strings.ToLower(host),
		Owner: parts[0],
		Repo:  parts[1],
	}, nil
}
