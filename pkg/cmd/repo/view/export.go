// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

// exporter implements output.Exporter for *repos.Repo. Field names match
// gh's `gh repo view --json …` catalog 1:1 where applicable; shithub-only
// fields are dropped rather than renamed.
type exporter struct{}

func (exporter) Fields() []string { return exportableFields }

// exportableFields is the canonical projection catalog. Order matters for
// `--json` (no value) listings; keep alphabetical.
//
// `forkCount`/`stargazerCount`/`watcherCount` are the gh-compat aliases
// (B2): gh's `gh repo view --json` exposes those names, and ported
// scripts break without them. The legacy `forks`/`stargazers`/`watchers`
// stay populated for one release cycle.
// H9 (F2-14): the gh-canonical surface uses `is*` boolean naming and
// `diskUsage` for size. Add the aliases alongside the existing names
// so ported scripts get a match without dropping the originals.
// Server-side gaps (parent, languages, hasIssuesEnabled,
// hasWikiEnabled, merge-strategy toggles) are queued separately —
// we expose only fields the response already carries.
var exportableFields = []string{
	"archived",
	"createdAt",
	"defaultBranch",
	"description",
	"diskUsage",
	"fork",
	"forkCount",
	"forks",
	"fullName",
	"homepage",
	"id",
	"isArchived",
	"isFork",
	"isPrivate",
	"isTemplate",
	"language",
	"license",
	"name",
	"nameWithOwner",
	"nodeId",
	"openIssues",
	"owner",
	"pushedAt",
	"size",
	"stargazerCount",
	"stargazers",
	"topics",
	"updatedAt",
	"url",
	"visibility",
	"watcherCount",
	"watchers",
}

// Filter projects a *repos.Repo onto the export catalog as a map. Returns
// the whole map; the output package then narrows by user-requested fields.
func (exporter) Filter(v any) (any, error) {
	r, ok := v.(*repos.Repo)
	if !ok {
		return nil, fmt.Errorf("repo view exporter: want *repos.Repo, got %T", v)
	}
	license := map[string]any(nil)
	if r.License != nil {
		license = map[string]any{
			"key":  r.License.Key,
			"name": r.License.Name,
		}
		if r.License.SPDXID != "" {
			license["spdxId"] = r.License.SPDXID
		}
	}
	return map[string]any{
		"archived":      r.Archived,
		"createdAt":     r.CreatedAt,
		"defaultBranch": r.DefaultBranch,
		"description":   r.Description,
		"diskUsage":     r.Size,
		"fork":          r.Fork,
		"forkCount":     r.Forks,
		"forks":         r.Forks,
		"fullName":      r.FullName,
		"homepage":      r.Homepage,
		"id":            r.ID,
		"isArchived":    r.Archived,
		"isFork":        r.Fork,
		"isPrivate":     r.Private,
		"isTemplate":    r.IsTemplate,
		"language":      r.Language,
		"license":       license,
		"name":          r.Name,
		"nameWithOwner": r.FullName,
		// I7b (audit-I25): opaque opaque-id alongside the sequential id.
		"nodeId":         r.NodeID,
		"openIssues":     r.OpenIssues,
		"owner":          map[string]any{"login": r.Owner.Login, "type": r.Owner.Type},
		"pushedAt":       r.PushedAt,
		"size":           r.Size,
		"stargazerCount": r.Stargazers,
		"stargazers":     r.Stargazers,
		"topics":         r.Topics,
		"updatedAt":      r.UpdatedAt,
		"url":            r.HTMLURL,
		"visibility":     visibilityField(r),
		"watcherCount":   r.Watchers,
		"watchers":       r.Watchers,
	}, nil
}

// visibilityField normalizes the shithub envelope's two visibility hints
// into a single string. Server always sends "public" or "private" in
// `visibility`; the `private` boolean is the legacy alias.
func visibilityField(r *repos.Repo) string {
	if r.Visibility != "" {
		return r.Visibility
	}
	if r.Private {
		return "private"
	}
	return "public"
}
