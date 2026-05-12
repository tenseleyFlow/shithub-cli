// SPDX-License-Identifier: AGPL-3.0-or-later

package status

import (
	"strings"
	"time"

	searchshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/search/shared"
)

// sectionQueries holds the rendered query strings for the four
// dashboard sections. They're constructed once per Run from the user
// login and the org filter knobs.
type sectionQueries struct {
	assignedIssues string
	assignedPRs    string
	reviewRequests string
	mentions       string
}

// buildQueries lowers the user login + org filters into the four
// section query strings. Each shares the org / exclude qualifiers so
// `--org foo` consistently filters every section. The mentions section
// adds an `updated:>=YYYY-MM-DD` window (MentionsWindow) so we don't
// pull a user's entire mention history.
func buildQueries(login, org string, exclude []string, now time.Time) sectionQueries {
	common := commonQualifiers(org, exclude)

	cutoff := now.Add(-MentionsWindow).Format("2006-01-02")

	return sectionQueries{
		assignedIssues: searchshared.ComposeQuery(
			"",
			withCommon(
				common,
				searchshared.Qualifier{Key: "assignee", Value: login},
				searchshared.Qualifier{Key: "state", Value: "open"},
			)...,
		),
		assignedPRs: searchshared.ComposeQuery(
			"",
			withCommon(
				common,
				searchshared.Qualifier{Key: "assignee", Value: login},
				searchshared.Qualifier{Key: "state", Value: "open"},
			)...,
		),
		reviewRequests: searchshared.ComposeQuery(
			"",
			withCommon(
				common,
				searchshared.Qualifier{Key: "review-requested", Value: login},
				searchshared.Qualifier{Key: "state", Value: "open"},
			)...,
		),
		mentions: searchshared.ComposeQuery(
			"",
			withCommon(
				common,
				searchshared.Qualifier{Key: "mentions", Value: login},
				searchshared.Qualifier{Key: "updated", Value: ">=" + cutoff},
			)...,
		),
	}
}

// commonQualifiers returns the org / -org filters every section shares.
func commonQualifiers(org string, exclude []string) []searchshared.Qualifier {
	out := []searchshared.Qualifier{}
	if s := strings.TrimSpace(org); s != "" {
		out = append(out, searchshared.Qualifier{Key: "org", Value: s})
	}
	for _, x := range exclude {
		if s := strings.TrimSpace(x); s != "" {
			// gh's negation syntax: `-org:foo`. Server's parser mirrors
			// this — the Key carries the leading minus so ComposeQuery
			// emits `-org:foo` verbatim.
			out = append(out, searchshared.Qualifier{Key: "-org", Value: s})
		}
	}
	return out
}

// withCommon prepends the section-shared qualifiers to a per-section
// list. We could `append(common, extra...)` inline; having a helper
// keeps the buildQueries body readable.
func withCommon(common []searchshared.Qualifier, extra ...searchshared.Qualifier) []searchshared.Qualifier {
	out := make([]searchshared.Qualifier, 0, len(common)+len(extra))
	out = append(out, common...)
	out = append(out, extra...)
	return out
}
