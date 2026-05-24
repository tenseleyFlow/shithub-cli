// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	prshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/pr/shared"
)

// exporter projects each PR onto the gh-compatible JSON shape.
type exporter struct{}

func (exporter) Fields() []string { return exportableFields }

// ExportableFields returns the catalog used by both `pr list --json`
// and `pr view --json`. Promoted so the view package can stay in sync
// without duplicating the slice.
func ExportableFields() []string {
	return append([]string(nil), exportableFields...)
}

var exportableFields = []string{
	// I8 (audit-I14): the field catalog mirrors what `pr view --json`
	// exposes in gh. Stub-empty fields (latestReviews / projectItems /
	// statusCheckRollup / closingIssuesReferences / files) are documented
	// in the field-help text; they ship as deferred-feature placeholders
	// so ported gh scripts can probe for them without crashing.
	"additions",
	"assignees",
	"author",
	"authorAssociation",
	"baseRefName",
	"baseRefOid",
	"baseRepository",
	"body",
	"changedFiles",
	"closedAt",
	"closingIssuesReferences",
	"comments",
	"commits",
	"createdAt",
	"deletions",
	"displayState",
	"draft",
	"files",
	"headRefName",
	"headRefOid",
	"headRepository",
	"id",
	"isCrossRepository",
	"isDraft",
	"labels",
	"latestReviews",
	"mergeStateStatus",
	"mergeable",
	"merged",
	"mergedAt",
	"mergedBy",
	"milestone",
	"nodeId",
	"number",
	"projectItems",
	"reviewDecision",
	"reviewRequests",
	"state",
	"statusCheckRollup",
	"title",
	"updatedAt",
	"url",
}

func (exporter) Filter(v any) (any, error) {
	list, ok := v.([]pulls.PR)
	if !ok {
		return nil, fmt.Errorf("pr list exporter: want []pulls.PR, got %T", v)
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, ProjectPR(p))
	}
	return out, nil
}

// ProjectPR is shared with `pr view` so single-PR and listing exports
// emit identical field names.
//
// E-audit E2 added the four ref-OID + repo fields gh-compat clients
// expect: baseRefOid, headRefOid, baseRepository, headRepository. The
// `isCrossRepository` derived field surfaces whether the PR comes
// from a fork — a one-liner that needed both halves of E2 (server
// nesting the repo envelope + this exporter wiring) to work.
func ProjectPR(p pulls.PR) map[string]any {
	var author any
	if p.User != nil {
		author = map[string]any{"login": p.User.Login}
	}
	labels := make([]map[string]any, 0, len(p.Labels))
	for _, l := range p.Labels {
		labels = append(labels, map[string]any{"name": l.Name, "color": l.Color})
	}
	// I8 (audit-I14): assignees/mergedBy/milestone/reviewRequests
	// project through the user envelope helpers below; review requests
	// fold reviewers + teams into one gh-compat shape.
	assignees := make([]map[string]any, 0, len(p.Assignees))
	for _, a := range p.Assignees {
		login := a.Login
		assignees = append(assignees, map[string]any{"login": login})
	}
	reviewRequests := make([]map[string]any, 0, len(p.RequestedRevs))
	for _, r := range p.RequestedRevs {
		reviewRequests = append(reviewRequests, map[string]any{"login": r.Login})
	}
	var mergedBy any
	if p.MergedBy != nil {
		mergedBy = map[string]any{"login": p.MergedBy.Login}
	}
	var milestone any
	if p.Milestone != nil {
		milestone = map[string]any{
			"number": p.Milestone.Number,
			"title":  p.Milestone.Title,
		}
	}
	return map[string]any{
		"author":         author,
		"baseRefName":    p.Base.Ref,
		"baseRefOid":     p.Base.SHA,
		"baseRepository": repoLiteAsExport(p.Base.Repo),
		"body":           p.Body,
		"closedAt":       p.ClosedAt,
		"comments":       p.Comments,
		"createdAt":      p.CreatedAt,
		// audit-I39: human-formatter projection of the state machine.
		// `state` stays gh-canonical ("open"/"closed"), `merged` is a
		// separate boolean, and `displayState` is what `pr list` /
		// `pr view` print in the table (merged > closed > draft > open).
		// Scripts can pick either depending on whether they want raw
		// state or display-equivalent.
		"displayState":      prshared.DisplayState(&p),
		"draft":             p.Draft,
		"headRefName":       p.Head.Ref,
		"headRefOid":        p.Head.SHA,
		"headRepository":    repoLiteAsExport(p.Head.Repo),
		"id":                p.ID,
		"isCrossRepository": isCrossRepository(p),
		"isDraft":           p.Draft,
		"labels":            labels,
		"mergeStateStatus":  p.MergeableState,
		"mergeable":         mergeableFlag(p.Mergeable),
		"merged":            p.Merged,
		"mergedAt":          p.MergedAt,
		// I7b (audit-I25): opaque opaque-id alongside the sequential
		// id; prefer for scripts going forward.
		"nodeId":         p.NodeID,
		"number":         p.Number,
		"reviewDecision": p.ReviewDecision,
		"state":          p.State,
		"title":          p.Title,
		"updatedAt":      p.UpdatedAt,
		"url":            p.HTMLURL,
		// I8 (audit-I14): gh-canonical fields from I7a server expansion.
		"additions":         p.Additions,
		"assignees":         assignees,
		"authorAssociation": p.AuthorAssociation,
		"changedFiles":      p.ChangedFiles,
		"commits":           p.Commits,
		"deletions":         p.Deletions,
		"mergedBy":          mergedBy,
		"milestone":         milestone,
		"reviewRequests":    reviewRequests,
		// Deferred-feature stubs. Each returns an empty value of the
		// gh-canonical shape so ported scripts can probe-and-skip
		// rather than crashing on a missing key. Wiring lands when the
		// underlying feature ships (reactions, projects, status
		// checks, linked-issue parser, file-list lazy fetch).
		"closingIssuesReferences": []any{},
		"files":                   []any{},
		"latestReviews":           []any{},
		"projectItems":            []any{},
		"statusCheckRollup":       map[string]any{"state": "UNKNOWN"},
	}
}

// repoLiteAsExport renders the base/head repo node in the shape
// gh-compat clients consume. Returns nil when the server didn't
// populate the node (graceful degradation for older servers).
func repoLiteAsExport(r *pulls.RepoLite) any {
	if r == nil {
		return nil
	}
	var owner any
	if r.Owner != nil {
		owner = map[string]any{"login": r.Owner.Login}
	}
	return map[string]any{
		"id":        r.ID,
		"name":      r.Name,
		"full_name": r.FullName,
		"owner":     owner,
		"private":   r.Private,
		"url":       r.HTMLURL,
	}
}

// mergeableFlag dereferences the optional Mergeable pointer. nil maps
// to nil (unknown), preserving the three-state semantics gh emits as
// "UNKNOWN"/"CONFLICTING"/"MERGEABLE" — we just emit the raw bool
// since the server hasn't normalized to the enum form yet.
func mergeableFlag(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

// isCrossRepository reports whether head and base live on different
// repos (fork PRs). True when both repos are present and their
// full_name differs; conservative false otherwise.
func isCrossRepository(p pulls.PR) bool {
	if p.Base.Repo == nil || p.Head.Repo == nil {
		return false
	}
	return p.Base.Repo.FullName != p.Head.Repo.FullName
}
