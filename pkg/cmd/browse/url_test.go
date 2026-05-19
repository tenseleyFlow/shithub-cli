// SPDX-License-Identifier: AGPL-3.0-or-later

package browse

import (
	"testing"

	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		in     string
		target Target
		want   Components
	}{
		{"", TargetNone, Components{}},
		{"123", TargetNumber, Components{Number: 123}},
		{"pr/45", TargetNumber, Components{Number: 45, Hint: "pr"}},
		{"issue/9", TargetNumber, Components{Number: 9, Hint: "issue"}},
		{"deadbeef", TargetSHA, Components{SHA: "deadbeef"}},
		{"dev:src/main.go", TargetBranchPath, Components{Branch: "dev", Path: "src/main.go"}},
		{"feature/x:cmd/main.go", TargetBranchPath, Components{Branch: "feature/x", Path: "cmd/main.go"}},
		{"/cmd/main.go", TargetPath, Components{Path: "cmd/main.go"}},
		{"README.md", TargetPath, Components{Path: "README.md"}},
	}
	for _, tc := range cases {
		gotT, gotC := Classify(tc.in)
		if gotT != tc.target {
			t.Errorf("Classify(%q) target: got %d want %d", tc.in, gotT, tc.target)
			continue
		}
		if gotC != tc.want {
			t.Errorf("Classify(%q) components: got %+v want %+v", tc.in, gotC, tc.want)
		}
	}
}

func TestComposeEmpty(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, err := Compose(TargetNone, Components{}, ComposeOptions{Repo: repo})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if url != "https://shithub.sh/o/r" {
		t.Errorf("url: %q", url)
	}
}

func TestComposeNumberPrefersPR(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetNumber, Components{Number: 42}, ComposeOptions{Repo: repo})
	// G4 (F32): plural `/pulls/{N}` — shithub's web app 404s on the
	// singular `/pull/{N}` form gh uses.
	if url != "https://shithub.sh/o/r/pulls/42" {
		t.Errorf("default number → pulls: %q", url)
	}
}

// G4 (F32): browse with "pr" hint must emit the plural `/pulls/{N}` route.
// Pinned alongside the default case because both paths shared the same
// pre-fix bug.
func TestComposeNumberPRHintEmitsPlural(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetNumber, Components{Number: 7, Hint: "pr"}, ComposeOptions{Repo: repo})
	if url != "https://shithub.sh/o/r/pulls/7" {
		t.Errorf("pr hint: %q want https://shithub.sh/o/r/pulls/7", url)
	}
}

func TestComposeNumberIssueHint(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetNumber, Components{Number: 9, Hint: "issue"}, ComposeOptions{Repo: repo})
	if url != "https://shithub.sh/o/r/issues/9" {
		t.Errorf("issue hint: %q", url)
	}
}

func TestComposeSHA(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetSHA, Components{SHA: "abc1234"}, ComposeOptions{Repo: repo})
	if url != "https://shithub.sh/o/r/commit/abc1234" {
		t.Errorf("sha: %q", url)
	}
}

func TestComposeBranchPath(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetBranchPath, Components{Branch: "dev", Path: "src/main.go"}, ComposeOptions{Repo: repo})
	if url != "https://shithub.sh/o/r/blob/dev/src/main.go" {
		t.Errorf("branch:path: %q", url)
	}
}

func TestComposePathDefaultsHEAD(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetPath, Components{Path: "README.md"}, ComposeOptions{Repo: repo})
	if url != "https://shithub.sh/o/r/blob/HEAD/README.md" {
		t.Errorf("path: %q", url)
	}
}

func TestComposePathWithBranchOverride(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetPath, Components{Path: "README.md"}, ComposeOptions{Repo: repo, Branch: "dev"})
	if url != "https://shithub.sh/o/r/blob/dev/README.md" {
		t.Errorf("--branch: %q", url)
	}
}

func TestComposeTreeWithBranchOnly(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetNone, Components{}, ComposeOptions{Repo: repo, Branch: "feature/x"})
	if url != "https://shithub.sh/o/r/tree/feature/x" {
		t.Errorf("tree branch: %q", url)
	}
}

func TestComposeTabs(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	for tab, suffix := range map[Tab]string{
		TabProjects: "/projects",
		TabReleases: "/releases",
		TabSettings: "/settings",
		TabWiki:     "/wiki",
	} {
		url, _ := Compose(TargetNone, Components{}, ComposeOptions{Repo: repo, Tab: tab})
		if url != "https://shithub.sh/o/r"+suffix {
			t.Errorf("tab %q: got %q", tab, url)
		}
	}
}

func TestComposeEscapesBranchSegments(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	url, _ := Compose(TargetBranchPath, Components{Branch: "feat#1", Path: "foo bar/baz"}, ComposeOptions{Repo: repo})
	if url != "https://shithub.sh/o/r/blob/feat%231/foo%20bar/baz" {
		t.Errorf("escaped url: %q", url)
	}
}

func TestComposeRefusesEmptyRepo(t *testing.T) {
	if _, err := Compose(TargetNone, Components{}, ComposeOptions{}); err == nil {
		t.Fatal("expected error for empty repo")
	}
}
