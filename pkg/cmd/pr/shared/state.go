// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import "github.com/tenseleyFlow/shithub-cli/internal/pulls"

// DisplayState returns the user-facing badge for a PR.
//
// audit-I7: pre-fix the badge was computed inconsistently across
// `pr view`, `pr list`, and `pr status`. `pr view` checked Draft
// before Merged so draft+merged showed "merged" (correct) but
// closed+draft showed "draft" (LIE — the PR is closed). `pr list`
// flipped the order so merged+draft would have shown "draft"
// (different LIE in a different command). Either way the displayed
// state contradicted the real state machine and hid the closed-ness.
//
// Precedence (highest wins):
//
//	merged   — pr.Merged or pr.MergedAt non-nil
//	closed   — pr.State == "closed"  (also covers closed-draft)
//	draft    — pr.Draft && pr.State == "open"
//	open     — pr.State == "open" && !pr.Draft
//
// audit-I39 piggyback: the JSON exporter projects the raw `state`
// field unchanged (so scripts see `"closed"` for merged PRs, with
// `merged: true` available separately). DisplayState is for human
// rendering only — never embed in --json output without adding a
// dedicated `displayState` field.
func DisplayState(pr *pulls.PR) string {
	if pr == nil {
		return ""
	}
	if pr.Merged || pr.MergedAt != nil {
		return "merged"
	}
	if pr.State == "closed" {
		return "closed"
	}
	if pr.Draft {
		return "draft"
	}
	return pr.State
}
