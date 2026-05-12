// SPDX-License-Identifier: AGPL-3.0-or-later

package browse

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

// Target classifies what the user typed as a positional argument. The
// classifier is purely lexical — no API calls — so callers can decide
// whether to disambiguate ambiguous shapes (e.g., a bare number that
// could be a PR or an issue) via lazy resolution.
type Target int

const (
	// TargetNone is the empty-arg case (open repo home).
	TargetNone Target = iota
	// TargetNumber is a bare digit string (issue or PR — caller decides).
	TargetNumber
	// TargetSHA is a 7–40 char hex string (commit view).
	TargetSHA
	// TargetBranchPath is "branch:path/to/file".
	TargetBranchPath
	// TargetPath is "path/to/file" (resolved against default branch).
	TargetPath
)

// Classify inspects `arg` and returns its Target type plus parsed
// components. The string forms recognized:
//
//	""                    → TargetNone
//	"123"                 → TargetNumber (number=123)
//	"deadbeef" / 40 hex   → TargetSHA (sha=arg)
//	"branch:path/to/file" → TargetBranchPath (branch, path)
//	"path/to/file"        → TargetPath (path)
//	"pr/123" / "issue/45" → TargetNumber with PR/issue hint via the
//	                        returned prefix (callers branch on it)
//
// The hint (prefix) is "pr", "issue", or "" — opaque to the classifier
// but used by `browse` to skip the auto-detect roundtrip.
func Classify(arg string) (Target, Components) {
	s := strings.TrimSpace(arg)
	if s == "" {
		return TargetNone, Components{}
	}

	// Explicit hint forms.
	for _, prefix := range []string{"pr", "issue"} {
		if rest, ok := strings.CutPrefix(s, prefix+"/"); ok {
			if n, err := strconv.Atoi(rest); err == nil && n > 0 {
				return TargetNumber, Components{Number: n, Hint: prefix}
			}
		}
	}

	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return TargetNumber, Components{Number: n}
	}

	if isHexSHA(s) {
		return TargetSHA, Components{SHA: s}
	}

	if branch, path, ok := strings.Cut(s, ":"); ok && branch != "" && path != "" {
		return TargetBranchPath, Components{Branch: branch, Path: cleanPath(path)}
	}

	return TargetPath, Components{Path: cleanPath(s)}
}

// Components carries the parsed pieces returned alongside a Target. Most
// fields are zero unless the target type populates them.
type Components struct {
	Number int
	SHA    string
	Branch string
	Path   string
	Hint   string // "pr" | "issue" | ""
}

// Tab is one of the explicit `--projects`/`--releases`/`--settings`/
// `--wiki` flags. Empty Tab means "no tab override; honor target".
type Tab string

// Tab values.
const (
	TabNone     Tab = ""
	TabProjects Tab = "projects"
	TabReleases Tab = "releases"
	TabSettings Tab = "settings"
	TabWiki     Tab = "wiki"
)

// ComposeOptions captures the runtime knobs the URL builder cares about.
// Repo is the destination (host/owner/name); Branch overrides the
// classifier's branch when set (used by `-b` against TargetPath /
// TargetNone).
type ComposeOptions struct {
	Repo   repocmdshared.RepoRef
	Tab    Tab
	Branch string
}

// Compose builds the browser URL for the given target + options.
// Returns an error when the inputs don't form a valid shithub URL
// (e.g., empty repo, conflicting flags).
func Compose(target Target, comp Components, opts ComposeOptions) (string, error) {
	if opts.Repo.Owner == "" || opts.Repo.Name == "" {
		return "", errors.New("browse: repo is required")
	}
	base := repocmdshared.WebURL(opts.Repo)

	switch opts.Tab {
	case TabProjects:
		return base + "/projects", nil
	case TabReleases:
		return base + "/releases", nil
	case TabSettings:
		return base + "/settings", nil
	case TabWiki:
		return base + "/wiki", nil
	}

	switch target {
	case TargetNone:
		if opts.Branch != "" {
			return base + "/tree/" + escapeRef(opts.Branch), nil
		}
		return base, nil
	case TargetNumber:
		// gh maps a bare number to PR first, then issue. We emit /issues/N
		// when the hint says "issue"; otherwise prefer /pull/N (shithub web
		// route also accepts /issues/N for PRs as a fallback).
		switch comp.Hint {
		case "issue":
			return fmt.Sprintf("%s/issues/%d", base, comp.Number), nil
		case "pr":
			return fmt.Sprintf("%s/pull/%d", base, comp.Number), nil
		default:
			return fmt.Sprintf("%s/pull/%d", base, comp.Number), nil
		}
	case TargetSHA:
		return fmt.Sprintf("%s/commit/%s", base, comp.SHA), nil
	case TargetBranchPath:
		return fmt.Sprintf("%s/blob/%s/%s", base, escapeRef(comp.Branch), escapePath(comp.Path)), nil
	case TargetPath:
		branch := opts.Branch
		if branch == "" {
			branch = "HEAD" // shithub web route resolves HEAD to default branch
		}
		return fmt.Sprintf("%s/blob/%s/%s", base, escapeRef(branch), escapePath(comp.Path)), nil
	}
	return "", fmt.Errorf("browse: unsupported target %d", target)
}

// isHexSHA reports whether s looks like a commit SHA. We accept 7–40
// lowercase / uppercase hex characters — narrow enough to avoid catching
// short numeric tokens (which would already match TargetNumber first).
func isHexSHA(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// cleanPath trims a leading slash and runs path.Clean-style normalization
// without dragging the stdlib path package in for one call. Multiple
// slashes are collapsed; "./" and "../" segments stay intact — the
// server handles those.
func cleanPath(p string) string {
	p = strings.TrimPrefix(p, "/")
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

// escapeRef URL-encodes a branch / ref name. Branches can contain "/"
// characters which we must keep as path separators, so we escape each
// segment individually.
func escapeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// escapePath URL-encodes a file path while preserving "/" separators.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/")
}
