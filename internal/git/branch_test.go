// SPDX-License-Identifier: AGPL-3.0-or-later

package git

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// commitFile writes a file and creates a commit. Returns the resulting
// HEAD SHA so callers can assert on it.
func commitFile(t *testing.T, r Runner, dir, name, content, message string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := r.Run(dir, []string{"add", name}, io.Discard, io.Discard); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := r.Run(dir, []string{"commit", "-m", message}, io.Discard, io.Discard); err != nil {
		t.Fatalf("commit: %v", err)
	}
	sha, err := HeadSHA(r, dir, "HEAD")
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	return sha
}

func TestBranchExistsAndCheckoutNew(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")

	yes, _ := BranchExists(r, dir, "trunk")
	if !yes {
		t.Error("trunk should exist")
	}
	no, _ := BranchExists(r, dir, "feature")
	if no {
		t.Error("feature should not exist yet")
	}

	if err := CheckoutNewBranch(r, dir, "feature", "", "trunk", io.Discard, io.Discard); err != nil {
		t.Fatalf("CheckoutNewBranch: %v", err)
	}
	branch, _ := CurrentBranch(r, dir)
	if branch != "feature" {
		t.Errorf("current branch: %q", branch)
	}
}

func TestLogSubjectsAndBody(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	_ = commitFile(t, r, dir, "a", "1", "subject A\n\nbody A line 1\nbody A line 2")
	_ = commitFile(t, r, dir, "b", "2", "subject B")

	subjects, err := LogSubjects(r, dir, "HEAD~2..HEAD")
	if err != nil {
		t.Fatalf("LogSubjects: %v", err)
	}
	if len(subjects) != 2 || subjects[0] != "subject A" || subjects[1] != "subject B" {
		t.Errorf("subjects: %v", subjects)
	}

	body, err := LogBody(r, dir, "HEAD~2..HEAD")
	if err != nil {
		t.Fatalf("LogBody: %v", err)
	}
	if !strings.Contains(body, "body A line 1") {
		t.Errorf("body: %q", body)
	}
}

func TestCommitsAheadBehindHandlesMissingRef(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	ahead, behind, err := CommitsAheadBehind(r, dir, "trunk", "origin/trunk")
	if err != nil {
		t.Fatalf("CommitsAheadBehind: %v", err)
	}
	if ahead != 0 || behind != 0 {
		t.Errorf("missing-ref should yield zeros: %d/%d", ahead, behind)
	}
}

func TestCommitsAheadBehindBetweenLocalRefs(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	_ = commitFile(t, r, dir, "a", "1", "A")
	if err := CheckoutNewBranch(r, dir, "feature", "", "HEAD", io.Discard, io.Discard); err != nil {
		t.Fatalf("CheckoutNewBranch: %v", err)
	}
	_ = commitFile(t, r, dir, "b", "2", "B")

	ahead, behind, err := CommitsAheadBehind(r, dir, "feature", "trunk")
	if err != nil {
		t.Fatalf("CommitsAheadBehind: %v", err)
	}
	if ahead != 1 || behind != 0 {
		t.Errorf("ahead/behind: got %d/%d want 1/0", ahead, behind)
	}
}

func TestHeadSHA(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	sha, err := HeadSHA(r, dir, "")
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-hex SHA, got %q", sha)
	}
}

func TestUpstreamOfReturnsEmptyWhenUnconfigured(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	got, err := UpstreamOf(r, dir, "trunk")
	if err != nil {
		t.Fatalf("UpstreamOf: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty for non-tracked branch, got %q", got)
	}
}

func TestCheckoutDetached(t *testing.T) {
	r, dir := newTestRepo(t, "trunk")
	sha, _ := HeadSHA(r, dir, "")
	if err := CheckoutDetached(r, dir, sha, io.Discard, io.Discard); err != nil {
		t.Fatalf("CheckoutDetached: %v", err)
	}
	if _, err := CurrentBranch(r, dir); err == nil {
		t.Error("expected error from CurrentBranch after detached HEAD")
	}
}
