// SPDX-License-Identifier: AGPL-3.0-or-later

// Package shared holds helpers used across the `shithub repo` subcommands —
// repo resolution from `-R`, current git remote, or `.git/config`'s
// shithub.default-repo key. Keeping this out of each subcommand avoids
// drift when we tighten the resolution rules.
package shared

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
)

// RepoRef names a single shithub repository. Host can be empty when the
// caller hasn't decided which host to target yet (the cmdutil.Factory's
// DefaultHost resolves it later).
type RepoRef struct {
	Host  string
	Owner string
	Name  string
}

// String renders "host/owner/name" when host is set, otherwise "owner/name".
// Used for diagnostic output, not URL composition.
func (r RepoRef) String() string {
	if r.Host == "" {
		return r.Owner + "/" + r.Name
	}
	return r.Host + "/" + r.Owner + "/" + r.Name
}

// FullName returns "owner/name". Most API calls take this format.
func (r RepoRef) FullName() string {
	return r.Owner + "/" + r.Name
}

// ParseRepoArg accepts the canonical `<owner>/<name>` form, `host/owner/name`,
// HTTPS/SSH URL forms, and tolerates a trailing `.git` suffix on any of
// them (C-audit C16+C17). gh-compat: `gh issue list -R https://github.com/cli/cli`
// works, and copy-pasting `git remote get-url origin` into `-R` works.
func ParseRepoArg(s string) (RepoRef, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return RepoRef{}, errors.New("repo: argument is empty")
	}

	// URL forms: route through the existing ParseRemoteURL which handles
	// https://, http://, ssh://, and the SCP-like git@host:owner/repo
	// form. ParseRemoteURL strips the `.git` suffix already.
	if isLikelyURL(s) {
		rem, err := git.ParseRemoteURL(s)
		if err != nil {
			return RepoRef{}, fmt.Errorf("repo: parse %q: %w", s, err)
		}
		return RepoRef{Host: rem.Host, Owner: rem.Owner, Name: rem.Repo}, nil
	}

	// Bare owner/name (or host/owner/name) form. Strip a trailing
	// `.git` from the repo segment so scripts that paste the literal
	// remote-URL tail (`mfwolffe/repo.git`) don't 404.
	s = strings.TrimSuffix(s, ".git")
	parts := strings.Split(s, "/")
	switch len(parts) {
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return RepoRef{}, fmt.Errorf("repo: expected owner/name, got %q", s)
		}
		return RepoRef{Owner: parts[0], Name: parts[1]}, nil
	case 3:
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return RepoRef{}, fmt.Errorf("repo: expected host/owner/name, got %q", s)
		}
		// G12 (F17 / F25): a 3-part input must look like host/owner/name —
		// the first segment needs to plausibly be a hostname so we don't
		// accept "owner/repo/extra" as host="owner" and then surface the
		// downstream "no token configured for host: owner" red herring.
		// Conservative heuristic: a real host has a "." (or is the bare
		// "localhost"); bare alphanumeric strings can't be hostnames.
		if !looksLikeHost(parts[0]) {
			return RepoRef{}, fmt.Errorf("repo: expected owner/name (got %q)", s)
		}
		return RepoRef{Host: parts[0], Owner: parts[1], Name: parts[2]}, nil
	default:
		return RepoRef{}, fmt.Errorf("repo: expected owner/name (got %q)", s)
	}
}

// looksLikeHost reports whether s plausibly names a hostname. Used by
// the 3-part repo-arg parser to reject "owner/repo/extra" before it
// downgrades to a doomed auth lookup against a bogus host.
func looksLikeHost(s string) bool {
	if s == "localhost" {
		return true
	}
	return strings.Contains(s, ".")
}

// isLikelyURL reports whether s looks like a remote URL rather than a
// bare owner/name pair. Conservative: only the shapes ParseRemoteURL
// actually accepts trigger the URL path, so users with a literal `:`
// or `@` in an owner segment still hit the bare path with a clear
// error.
func isLikelyURL(s string) bool {
	if strings.Contains(s, "://") {
		return true
	}
	// SCP-like: user@host:path
	if at := strings.IndexByte(s, '@'); at > 0 {
		if colon := strings.IndexByte(s, ':'); colon > at {
			return true
		}
	}
	return false
}

// Resolver is the contract a command uses to figure out which repo it's
// acting on. Production wires this from the Factory; tests inject a fake
// to avoid touching the filesystem.
type Resolver struct {
	// RepoFlag is the value of the -R/--repo flag (may be empty).
	RepoFlag string
	// Hostname is the value of --hostname (may be empty).
	Hostname string
	// DefaultHost is the host to fall back to when nothing else specifies one.
	DefaultHost string
	// GitRunner is used to read the local repo's remote/config when the
	// flag isn't set. May be nil when callers don't want filesystem
	// fallback (e.g., `repo create` when no flag arg given).
	GitRunner git.Runner
	// Dir is the working dir to consult for git operations (empty = cwd).
	Dir string
	// RemoteName is the git remote name to consult; defaults to "origin"
	// when empty.
	RemoteName string
}

// Resolve returns the RepoRef to operate on, applying this precedence:
//
//  1. -R / --repo flag value (parsed via ParseRepoArg).
//  2. `git config --get shithub.default-repo` (set by `repo set-default`).
//  3. `git remote get-url <RemoteName>` parsed via internal/git.ParseRemoteURL.
//
// Host is filled from the parsed value when present, otherwise from
// Hostname / DefaultHost (in that order). Returns an error when none of
// the sources yield a repo.
func (r Resolver) Resolve() (RepoRef, error) {
	if strings.TrimSpace(r.RepoFlag) != "" {
		ref, err := ParseRepoArg(r.RepoFlag)
		if err != nil {
			return RepoRef{}, err
		}
		ref.Host = r.pickHost(ref.Host)
		return ref, nil
	}

	if r.GitRunner != nil {
		if ref, ok, err := r.fromGitConfig(); err != nil {
			return RepoRef{}, err
		} else if ok {
			ref.Host = r.pickHost(ref.Host)
			return ref, nil
		}
		if ref, ok, err := r.fromGitRemote(); err != nil {
			return RepoRef{}, err
		} else if ok {
			ref.Host = r.pickHost(ref.Host)
			return ref, nil
		}
	}

	return RepoRef{}, errors.New("repo: not specified; pass -R owner/repo, run inside a shithub clone, or set shithub.default-repo")
}

// pickHost applies the host-precedence rule: parsed > Hostname flag >
// DefaultHost > package default. Normalized through config.NormalizeHost
// so trailing slashes / mixed case don't sneak through.
func (r Resolver) pickHost(parsed string) string {
	if parsed != "" {
		return config.NormalizeHost(parsed)
	}
	if r.Hostname != "" {
		return config.NormalizeHost(r.Hostname)
	}
	if r.DefaultHost != "" {
		return config.NormalizeHost(r.DefaultHost)
	}
	return config.DefaultHost
}

// fromGitConfig reads `shithub.default-repo` and parses it. Missing key
// returns (zero, false, nil); a malformed value returns (zero, false, err).
func (r Resolver) fromGitConfig() (RepoRef, bool, error) {
	raw, err := git.GetConfig(r.GitRunner, r.Dir, "shithub.default-repo")
	if err != nil {
		return RepoRef{}, false, err
	}
	if raw == "" {
		return RepoRef{}, false, nil
	}
	// Allow the host:owner/repo form alongside owner/repo so multi-host
	// users can pin a default to a non-default host.
	if h, rest, ok := strings.Cut(raw, ":"); ok && !strings.Contains(h, "/") {
		ref, err := ParseRepoArg(rest)
		if err != nil {
			return RepoRef{}, false, fmt.Errorf("shithub.default-repo: %w", err)
		}
		ref.Host = h
		return ref, true, nil
	}
	ref, err := ParseRepoArg(raw)
	if err != nil {
		return RepoRef{}, false, fmt.Errorf("shithub.default-repo: %w", err)
	}
	return ref, true, nil
}

// fromGitRemote reads the named git remote and parses its URL into a
// RepoRef. Missing remote returns (zero, false, nil); a present-but-
// unparseable URL returns an error.
func (r Resolver) fromGitRemote() (RepoRef, bool, error) {
	name := r.RemoteName
	if name == "" {
		name = "origin"
	}
	has, err := git.RemoteExists(r.GitRunner, r.Dir, name)
	if err != nil || !has {
		return RepoRef{}, false, nil
	}
	remote, err := git.ResolveRemote(r.Dir, name)
	if err != nil {
		// Don't propagate parse errors as fatal; the caller might still
		// have other resolution paths (set-default config) — but in
		// practice fromGitConfig ran first, so just signal "not resolved".
		return RepoRef{}, false, nil
	}
	return RepoRef{Host: remote.Host, Owner: remote.Owner, Name: remote.Repo}, true, nil
}

// CloneURL composes the URL appropriate for `git clone` given the user's
// preferred protocol. Falls back to https when protocol is unrecognized.
func CloneURL(ref RepoRef, protocol string) string {
	host := ref.Host
	if host == "" {
		host = config.DefaultHost
	}
	switch strings.ToLower(protocol) {
	case "ssh":
		return fmt.Sprintf("git@%s:%s/%s.git", host, ref.Owner, ref.Name)
	default:
		return fmt.Sprintf("https://%s/%s/%s.git", host, ref.Owner, ref.Name)
	}
}

// WebURL returns the browser-visible URL for the repo's homepage on the
// host. Used by `repo view --web` and friends.
func WebURL(ref RepoRef) string {
	host := ref.Host
	if host == "" {
		host = config.DefaultHost
	}
	return fmt.Sprintf("https://%s/%s/%s", host, ref.Owner, ref.Name)
}
