// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

// TestProjectPR_E2Fields is the E2 CLI-side regression. The four
// fields landed on the exporter only after the server (PR #341)
// started nesting `repo` under PR base/head. Pin the catalog so a
// future re-trim doesn't silently strip them.
func TestProjectPR_E2Fields(t *testing.T) {
	wantFields := []string{
		"baseRefOid", "headRefOid",
		"baseRepository", "headRepository",
		"isCrossRepository",
	}
	catalog := map[string]bool{}
	for _, f := range ExportableFields() {
		catalog[f] = true
	}
	for _, f := range wantFields {
		if !catalog[f] {
			t.Errorf("ExportableFields missing %q (E2)", f)
		}
	}
}

// TestProjectPR_PopulatedRepos exercises the value-projection path:
// a same-repo PR renders matching base/head repositories and
// isCrossRepository=false; a cross-repo PR flips the latter.
func TestProjectPR_PopulatedRepos(t *testing.T) {
	owner := &api.User{Login: "alice"}
	base := pulls.Ref{
		Ref: "trunk", SHA: "aaa",
		Repo: &pulls.RepoLite{ID: 1, Name: "demo", FullName: "alice/demo", Owner: owner},
	}
	head := pulls.Ref{
		Ref: "feature", SHA: "bbb",
		Repo: &pulls.RepoLite{ID: 1, Name: "demo", FullName: "alice/demo", Owner: owner},
	}
	got := ProjectPR(pulls.PR{Number: 1, Title: "x", Base: base, Head: head})
	if got["baseRefOid"] != "aaa" {
		t.Errorf("baseRefOid: %v", got["baseRefOid"])
	}
	if got["headRefOid"] != "bbb" {
		t.Errorf("headRefOid: %v", got["headRefOid"])
	}
	if got["isCrossRepository"] != false {
		t.Errorf("isCrossRepository on same-repo: got %v", got["isCrossRepository"])
	}
	if m, ok := got["baseRepository"].(map[string]any); !ok || m["full_name"] != "alice/demo" {
		t.Errorf("baseRepository: %+v", got["baseRepository"])
	}

	// Fork PR: head lives on a different repo.
	headFork := pulls.Ref{
		Ref: "feature", SHA: "bbb",
		Repo: &pulls.RepoLite{ID: 2, Name: "demo", FullName: "bob/demo", Owner: &api.User{Login: "bob"}},
	}
	gotFork := ProjectPR(pulls.PR{Number: 2, Title: "fork", Base: base, Head: headFork})
	if gotFork["isCrossRepository"] != true {
		t.Errorf("isCrossRepository on fork: got %v", gotFork["isCrossRepository"])
	}
}

// TestProjectPR_F4Fields pins F4: the gh-canonical `mergeable` +
// `mergeStateStatus` fields. The audit found `--json mergeable,
// mergeStateStatus` rejected even though the server-side A17/A20
// work already computes mergeable_state. Mergeable=*bool round-trips
// as a bare bool; MergeableState=string projects unchanged.
func TestProjectPR_F4Fields(t *testing.T) {
	for _, want := range []string{"mergeable", "mergeStateStatus"} {
		found := false
		for _, f := range ExportableFields() {
			if f == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ExportableFields missing %q (F4)", want)
		}
	}

	mergeableTrue := true
	got := ProjectPR(pulls.PR{
		Number:         1,
		Mergeable:      &mergeableTrue,
		MergeableState: "clean",
	})
	if got["mergeable"] != true {
		t.Errorf("mergeable: got %v want true", got["mergeable"])
	}
	if got["mergeStateStatus"] != "clean" {
		t.Errorf("mergeStateStatus: got %v want clean", got["mergeStateStatus"])
	}

	// Nil pointer → nil (unknown).
	gotNil := ProjectPR(pulls.PR{Number: 2})
	if gotNil["mergeable"] != nil {
		t.Errorf("nil Mergeable should project nil, got %v", gotNil["mergeable"])
	}
}

// TestProjectPR_DisplayState pins audit-I39: the exporter projects a
// `displayState` field so scripts can ask for the human badge without
// reconstructing the precedence (merged > closed > draft > open).
// Closed-draft (the I7 reproducer) is exercised twice — once for the
// exporter field, once to confirm `state` and `merged` are unchanged
// so existing consumers don't break.
func TestProjectPR_DisplayState(t *testing.T) {
	cases := []struct {
		name             string
		pr               pulls.PR
		wantDisplayState string
		wantState        string
		wantMerged       bool
	}{
		{"open", pulls.PR{State: "open"}, "open", "open", false},
		{"draft open", pulls.PR{State: "open", Draft: true}, "draft", "open", false},
		{"closed", pulls.PR{State: "closed"}, "closed", "closed", false},
		{
			name:             "closed draft (I7 reproducer)",
			pr:               pulls.PR{State: "closed", Draft: true},
			wantDisplayState: "closed",
			wantState:        "closed",
			wantMerged:       false,
		},
		{
			name:             "merged",
			pr:               pulls.PR{State: "closed", Merged: true},
			wantDisplayState: "merged",
			wantState:        "closed",
			wantMerged:       true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProjectPR(tc.pr)
			if got["displayState"] != tc.wantDisplayState {
				t.Errorf("displayState: got %v, want %q", got["displayState"], tc.wantDisplayState)
			}
			if got["state"] != tc.wantState {
				t.Errorf("state: got %v, want %q", got["state"], tc.wantState)
			}
			if got["merged"] != tc.wantMerged {
				t.Errorf("merged: got %v, want %v", got["merged"], tc.wantMerged)
			}
		})
	}

	// Catalog gate so a future trim can't silently drop displayState.
	catalog := map[string]bool{}
	for _, f := range ExportableFields() {
		catalog[f] = true
	}
	if !catalog["displayState"] {
		t.Error("ExportableFields missing displayState (I39)")
	}
}

// TestProjectPR_NilReposDegradeGracefully covers the older-server
// case where the response lacks the `repo` envelope entirely. The
// exporter must emit nil for the per-side repo fields and false for
// isCrossRepository rather than panicking.
func TestProjectPR_NilReposDegradeGracefully(t *testing.T) {
	got := ProjectPR(pulls.PR{
		Number: 3,
		Base:   pulls.Ref{Ref: "trunk", SHA: "aaa"},
		Head:   pulls.Ref{Ref: "feature", SHA: "bbb"},
	})
	if got["baseRepository"] != nil || got["headRepository"] != nil {
		t.Errorf("nil-repo: %+v", got)
	}
	if got["isCrossRepository"] != false {
		t.Errorf("isCrossRepository with nil repos: %v", got["isCrossRepository"])
	}
}
