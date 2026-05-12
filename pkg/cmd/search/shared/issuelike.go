// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import "fmt"

// IssueFlags captures the common flag surface shared by `search issues`
// and `search prs`. Archived / Locked use the gh-style `X` / `no-X`
// pair so callers stay aligned with `repo list` ergonomics.
type IssueFlags struct {
	State      string
	Author     string
	Assignee   string
	Label      []string
	Milestone  string
	Mentions   string
	Involves   string
	Commenter  string
	Repo       string
	Owner      string
	Language   string
	Archived   bool
	NoArchived bool
	Locked     bool
	NoLocked   bool
	NoComments bool
	Reactions  string
	Match      string
}

// BuildIssueQualifiers lowers IssueFlags into a search qualifier list.
// Caller appends additional qualifiers (e.g., type=issue / type=pr) on
// top via ComposeQuery — the wire-level `type` parameter is handled by
// the search.Client method choice, not by qualifiers.
func BuildIssueQualifiers(f IssueFlags) ([]Qualifier, error) {
	out := []Qualifier{
		{Key: "state", Value: f.State},
		{Key: "author", Value: f.Author},
		{Key: "assignee", Value: f.Assignee},
		{Key: "milestone", Value: f.Milestone},
		{Key: "mentions", Value: f.Mentions},
		{Key: "involves", Value: f.Involves},
		{Key: "commenter", Value: f.Commenter},
		{Key: "repo", Value: f.Repo},
		{Key: "user", Value: f.Owner},
		{Key: "language", Value: f.Language},
		{Key: "in", Value: f.Match},
	}
	out = append(out, QualifierAll("label", f.Label)...)

	switch {
	case f.Archived:
		out = append(out, Qualifier{Key: "archived", Value: "true"})
	case f.NoArchived:
		out = append(out, Qualifier{Key: "archived", Value: "false"})
	}
	switch {
	case f.Locked:
		out = append(out, Qualifier{Key: "is", Value: "locked"})
	case f.NoLocked:
		out = append(out, Qualifier{Key: "is", Value: "unlocked"})
	}
	if f.NoComments {
		out = append(out, Qualifier{Key: "comments", Value: "0"})
	}
	if f.Reactions != "" {
		r, err := ParseRange(f.Reactions)
		if err != nil {
			return nil, fmt.Errorf("--reactions: %w", err)
		}
		out = append(out, Qualifier{Key: "reactions", Value: r.String()})
	}
	return out, nil
}
