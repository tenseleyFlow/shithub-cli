// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
	"github.com/tenseleyFlow/shithub-cli/internal/git"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

func TestParsePRArgNumeric(t *testing.T) {
	fb := repocmdshared.RepoRef{Owner: "o", Name: "r"}
	got, err := ParsePRArg(context.Background(), nil, "42", fb)
	if err != nil {
		t.Fatalf("ParsePRArg: %v", err)
	}
	if got.Number != 42 || got.Repo != fb {
		t.Errorf("got %+v", got)
	}
}

func TestParsePRArgHashPrefix(t *testing.T) {
	fb := repocmdshared.RepoRef{Owner: "o", Name: "r"}
	got, err := ParsePRArg(context.Background(), nil, "#7", fb)
	if err != nil || got.Number != 7 {
		t.Errorf("got %+v err=%v", got, err)
	}
}

func TestParsePRArgURL(t *testing.T) {
	got, err := ParsePRArg(context.Background(), nil, "https://shithub.sh/octo/hello/pull/9", repocmdshared.RepoRef{})
	if err != nil {
		t.Fatalf("ParsePRArg: %v", err)
	}
	if got.Repo.Owner != "octo" || got.Repo.Name != "hello" || got.Number != 9 || got.Repo.Host != "shithub.sh" {
		t.Errorf("got %+v", got)
	}
}

func TestParsePRArgPullsURL(t *testing.T) {
	got, _, err := func() (PRRef, bool, error) {
		ref, err := ParsePRArg(context.Background(), nil, "https://shithub.sh/octo/hello/pulls/9", repocmdshared.RepoRef{})
		return ref, false, err
	}()
	if err != nil {
		t.Fatalf("ParsePRArg: %v", err)
	}
	if got.Number != 9 {
		t.Errorf("got %+v", got)
	}
}

func TestParsePRArgBranchLookup(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/repos/o/r/pulls", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]pulls.PR{
			{Number: 11, Head: pulls.Ref{Ref: "feature"}},
		})
	})
	c := pulls.NewClient(srv.NewClient())

	got, err := ParsePRArg(context.Background(), c, "feature", repocmdshared.RepoRef{Owner: "o", Name: "r"})
	if err != nil {
		t.Fatalf("ParsePRArg: %v", err)
	}
	if got.Number != 11 {
		t.Errorf("got %+v", got)
	}
}

func TestFindPRByBranchNotFound(t *testing.T) {
	srv := fakeapi.New(t)
	srv.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r/pulls", 200, []pulls.PR{})
	c := pulls.NewClient(srv.NewClient())
	_, err := FindPRByBranch(context.Background(), c, repocmdshared.RepoRef{Owner: "o", Name: "r"}, "nope")
	var nf NotFoundError
	if !errors.As(err, &nf) {
		t.Errorf("expected NotFoundError, got %v", err)
	}
}

func TestPRWebURL(t *testing.T) {
	ref := PRRef{Repo: repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}, Number: 9}
	if got := PRWebURL(ref); got != "https://shithub.sh/o/r/pull/9" {
		t.Errorf("PRWebURL: %q", got)
	}
}

func TestNewPRWebURLEncodesAndDrafts(t *testing.T) {
	repo := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	got := NewPRWebURL(repo, "trunk", "feature", "subj", "body", true)
	if !strings.Contains(got, "compare/trunk...feature") {
		t.Errorf("URL: %q", got)
	}
	if !strings.Contains(got, "draft=1") {
		t.Errorf("draft missing: %q", got)
	}
	if !strings.Contains(got, "title=subj") {
		t.Errorf("title missing: %q", got)
	}
}

func TestFillStandard(t *testing.T) {
	r, dir := mkRepoWithCommits(t,
		[]string{"first subject\n\nfirst body"},
		[]string{"second subject"},
		[]string{"third subject"},
	)

	title, body, err := Fill(r, dir, FillOptions{Mode: FillStandard, RevRange: "trunk..HEAD"})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if title != "first subject" {
		t.Errorf("title: %q", title)
	}
	if !strings.Contains(body, "second subject") || !strings.Contains(body, "third subject") {
		t.Errorf("body: %q", body)
	}
}

func TestFillFirst(t *testing.T) {
	r, dir := mkRepoWithCommits(t,
		[]string{"first subject\n\nfirst body line"},
		[]string{"second"},
	)

	title, body, err := Fill(r, dir, FillOptions{Mode: FillFirst, RevRange: "trunk..HEAD"})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if title != "first subject" {
		t.Errorf("title: %q", title)
	}
	if !strings.Contains(body, "first body line") {
		t.Errorf("body: %q", body)
	}
	if strings.Contains(body, "second") {
		t.Errorf("body should not include second commit: %q", body)
	}
}

func TestFillVerboseConcatenates(t *testing.T) {
	r, dir := mkRepoWithCommits(t,
		[]string{"a-sub\n\na-body"},
		[]string{"b-sub\n\nb-body"},
	)

	_, body, err := Fill(r, dir, FillOptions{Mode: FillVerbose, RevRange: "trunk..HEAD"})
	if err != nil {
		t.Fatalf("Fill: %v", err)
	}
	for _, want := range []string{"a-sub", "a-body", "b-sub", "b-body"} {
		if !strings.Contains(body, want) {
			t.Errorf("verbose body missing %q: %q", want, body)
		}
	}
}

func TestFillNoneReturnsEmpty(t *testing.T) {
	title, body, err := Fill(nil, "", FillOptions{Mode: FillNone})
	if err != nil || title != "" || body != "" {
		t.Errorf("FillNone: %q %q %v", title, body, err)
	}
}

// mkRepoWithCommits builds a tempdir-backed git repo with a base commit
// on `trunk` and the given commit messages applied on a `feature` branch
// (checked out at return). Tests pass `trunk..HEAD` as the rev range to
// see only the feature commits.
func mkRepoWithCommits(t *testing.T, messages ...[]string) (git.Runner, string) {
	t.Helper()
	r, err := git.FromPath()
	if err != nil {
		t.Skipf("git not on PATH: %v", err)
	}
	dir := t.TempDir()
	if err := git.Init(r, dir, "trunk"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, args := range [][]string{
		{"config", "user.email", "t@e"},
		{"config", "user.name", "t"},
	} {
		_ = r.Run(dir, args, io.Discard, io.Discard)
	}
	// Base commit on trunk so trunk..HEAD has something to compare against.
	if err := os.WriteFile(filepath.Join(dir, "BASE"), []byte("base"), 0o600); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := r.Run(dir, []string{"add", "BASE"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("add base: %v", err)
	}
	if err := r.Run(dir, []string{"commit", "-m", "base"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("commit base: %v", err)
	}
	// Branch off for the user's commits.
	if err := r.Run(dir, []string{"checkout", "-b", "feature"}, io.Discard, io.Discard); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	for i, msg := range messages {
		name := "f" + string(rune('a'+i))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := r.Run(dir, []string{"add", name}, io.Discard, io.Discard); err != nil {
			t.Fatalf("add: %v", err)
		}
		if err := r.Run(dir, []string{"commit", "-m", msg[0]}, io.Discard, io.Discard); err != nil {
			t.Fatalf("commit %s: %v", name, err)
		}
	}
	return r, dir
}
