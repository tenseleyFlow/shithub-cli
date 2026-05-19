// SPDX-License-Identifier: AGPL-3.0-or-later

// Package search owns the typed wire shape and helper client for
// shithub's /api/v1/search/* surface (S50 §5). Response envelopes match
// GitHub's REST contract verbatim:
//
//	{ "total_count": N, "incomplete_results": bool, "items": [...] }
//
// We parameterize the items via Go generics so the CLI gets typed
// results without each subcommand duplicating decode plumbing.
package search

import (
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

// Response is the generic envelope returned by every /search endpoint.
type Response[T any] struct {
	TotalCount        int  `json:"total_count"`
	IncompleteResults bool `json:"incomplete_results"`
	Items             []T  `json:"items"`
}

// CodeMatch is one entry from /search/code. shithub includes a short
// matched-line preview in TextMatches when available (S28's FTS already
// produces snippets — server just needs to forward them).
type CodeMatch struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	SHA        string      `json:"sha,omitempty"`
	URL        string      `json:"url,omitempty"`
	HTMLURL    string      `json:"html_url,omitempty"`
	Repository *repos.Repo `json:"repository,omitempty"`
	// Repo is the flat `owner/name` string shithub returns today.
	// `Repository` exists for forward-compat with a richer envelope,
	// but the renderer should prefer `Repo` (E-audit E11: the table
	// column was rendering empty because the server doesn't ship the
	// nested `repository` object yet).
	Repo        string      `json:"repo,omitempty"`
	Score       float64     `json:"score,omitempty"`
	TextMatches []TextMatch `json:"text_matches,omitempty"`
}

// CommitMatch is one entry from /search/commits.
type CommitMatch struct {
	SHA        string      `json:"sha"`
	URL        string      `json:"url,omitempty"`
	HTMLURL    string      `json:"html_url,omitempty"`
	Repository *repos.Repo `json:"repository,omitempty"`
	Commit     struct {
		Message string `json:"message"`
		Author  *struct {
			Name  string    `json:"name"`
			Email string    `json:"email,omitempty"`
			Date  time.Time `json:"date"`
		} `json:"author,omitempty"`
		Committer *struct {
			Name  string    `json:"name"`
			Email string    `json:"email,omitempty"`
			Date  time.Time `json:"date"`
		} `json:"committer,omitempty"`
	} `json:"commit"`
	Author    *api.User `json:"author,omitempty"`
	Committer *api.User `json:"committer,omitempty"`
	Score     float64   `json:"score,omitempty"`
}

// TextMatch is the optional snippet payload for code / commit results.
// shithub's FTS produces these via ts_headline; we surface them as-is.
type TextMatch struct {
	ObjectURL  string  `json:"object_url,omitempty"`
	ObjectType string  `json:"object_type,omitempty"`
	Property   string  `json:"property,omitempty"`
	Fragment   string  `json:"fragment,omitempty"`
	Matches    []Match `json:"matches,omitempty"`
}

// Match describes a highlighted span inside a TextMatch fragment.
type Match struct {
	Text    string `json:"text,omitempty"`
	Indices [2]int `json:"indices,omitempty"`
}

// RepoItem aliases repos.Repo so callers can write
// search.Response[search.RepoItem] without dragging the repos import
// through every consumer.
type RepoItem = repos.Repo

// IssueItem aliases issues.Issue for the same reason.
type IssueItem = issues.Issue

// PRItem aliases pulls.PR for the same reason.
type PRItem = pulls.PR

// CodeItem is the response item for /search/code (this package's
// CodeMatch — aliased for symmetry with the cross-package items).
type CodeItem = CodeMatch

// CommitItem is the response item for /search/commits.
type CommitItem = CommitMatch
