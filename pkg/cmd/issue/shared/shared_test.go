// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/api"
	"github.com/tenseleyFlow/shithub-cli/internal/api/fakeapi"
	repocmdshared "github.com/tenseleyFlow/shithub-cli/pkg/cmd/repo/shared"
)

func TestParseIssueArgNumeric(t *testing.T) {
	fb := repocmdshared.RepoRef{Owner: "o", Name: "r"}
	got, fromURL, err := ParseIssueArg("123", fb)
	if err != nil {
		t.Fatalf("ParseIssueArg: %v", err)
	}
	if fromURL || got.Number != 123 || got.Repo != fb {
		t.Errorf("unexpected: %+v fromURL=%v", got, fromURL)
	}
}

func TestParseIssueArgHashPrefix(t *testing.T) {
	fb := repocmdshared.RepoRef{Owner: "o", Name: "r"}
	got, _, err := ParseIssueArg("#42", fb)
	if err != nil {
		t.Fatalf("ParseIssueArg: %v", err)
	}
	if got.Number != 42 {
		t.Errorf("got %d", got.Number)
	}
}

func TestParseIssueArgRequiresRepoForNumeric(t *testing.T) {
	if _, _, err := ParseIssueArg("1", repocmdshared.RepoRef{}); err == nil {
		t.Fatal("expected error when fallback ref empty")
	}
}

func TestParseIssueArgURL(t *testing.T) {
	got, fromURL, err := ParseIssueArg("https://shithub.sh/octo/hello/issues/9", repocmdshared.RepoRef{})
	if err != nil {
		t.Fatalf("ParseIssueArg URL: %v", err)
	}
	if !fromURL {
		t.Error("expected fromURL")
	}
	if got.Repo.Owner != "octo" || got.Repo.Name != "hello" || got.Number != 9 || got.Repo.Host != "shithub.sh" {
		t.Errorf("got %+v", got)
	}
}

func TestParseIssueArgPullURL(t *testing.T) {
	got, _, err := ParseIssueArg("https://shithub.sh/octo/hello/pull/12", repocmdshared.RepoRef{})
	if err != nil {
		t.Fatalf("ParseIssueArg pull: %v", err)
	}
	if got.Number != 12 {
		t.Errorf("got %d", got.Number)
	}
}

func TestParseIssueArgRejectsJunk(t *testing.T) {
	if _, _, err := ParseIssueArg("not-a-number", repocmdshared.RepoRef{Owner: "o", Name: "r"}); err == nil {
		t.Fatal("expected error")
	}
	if _, _, err := ParseIssueArg("https://shithub.sh/foo", repocmdshared.RepoRef{}); err == nil {
		t.Fatal("expected error: URL too short")
	}
}

func TestSplitList(t *testing.T) {
	got := SplitList([]string{"a,b", " c "})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len: got %v want %v", got, want)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("[%d] got %q want %q", i, v, want[i])
		}
	}
}

func TestExpandMeSubstitutesAuthedUser(t *testing.T) {
	srv := fakeapi.New(t)
	calls := 0
	srv.RegisterJSON(http.MethodGet, "/api/v1/user", 200, api.User{Login: "mf"})
	srv.Handle(http.MethodGet, "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"login":"mf"}`))
	})

	got, err := ExpandMe(context.Background(), srv.NewClient(), []string{"@me", "octo"})
	if err != nil {
		t.Fatalf("ExpandMe: %v", err)
	}
	if got[0] != "mf" || got[1] != "octo" {
		t.Errorf("expand: %v", got)
	}
}

func TestExpandMeSkipsWireWhenAbsent(t *testing.T) {
	srv := fakeapi.New(t)
	srv.Handle(http.MethodGet, "/api/v1/user", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("ExpandMe should not call /user when @me absent")
	})
	got, err := ExpandMe(context.Background(), srv.NewClient(), []string{"octo"})
	if err != nil {
		t.Fatalf("ExpandMe: %v", err)
	}
	if len(got) != 1 || got[0] != "octo" {
		t.Errorf("got %v", got)
	}
}

func TestIssueWebURL(t *testing.T) {
	ref := IssueRef{Repo: repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}, Number: 7}
	if got := IssueWebURL(ref); got != "https://shithub.sh/o/r/issues/7" {
		t.Errorf("WebURL: %q", got)
	}
}

func TestNewIssueWebURLEncodesParams(t *testing.T) {
	ref := repocmdshared.RepoRef{Host: "shithub.sh", Owner: "o", Name: "r"}
	got := NewIssueWebURL(ref, "hello world", "body!")
	if !strings.Contains(got, "title=hello+world") {
		t.Errorf("title not encoded: %q", got)
	}
	if !strings.Contains(got, "body=body%21") {
		t.Errorf("body not encoded: %q", got)
	}
}
