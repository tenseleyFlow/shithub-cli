// SPDX-License-Identifier: AGPL-3.0-or-later

package edit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/repos"
)

func TestEditDescription(t *testing.T) {
	tf := cmdutiltest.New(t)
	var body json.RawMessage
	tf.Server.Handle(http.MethodPatch, "/api/v1/repos/o/r", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.Repo{
			Name: "r", FullName: "o/r", Owner: repos.Owner{Login: "o"}, DefaultBranch: "trunk",
		})
	})

	desc := "new"
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
		Description: &desc,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	if got["description"] != "new" {
		t.Errorf("desc not patched: %v", got)
	}
}

func TestEditReplaceTopics(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodPut, "/api/v1/repos/o/r/topics", 200, repos.TopicsPayload{Names: []string{"a", "b"}})

	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
		Topics:      []string{"a", "b"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tf.Server.AssertCalled(http.MethodPut, "/api/v1/repos/o/r/topics")
}

func TestEditAddRemoveTopicsMutate(t *testing.T) {
	tf := cmdutiltest.New(t)
	// E-audit E8: ListTopics now reads Topics from the repo view payload
	// (the server has no GET /topics endpoint — that returned 405 and
	// broke the RMW path). Register the repo view, not GET /topics.
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{
		Name: "r", Topics: []string{"go", "cli"},
	})

	var body json.RawMessage
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/topics", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = b
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.TopicsPayload{Names: []string{"go", "rest"}})
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		RepoArg:      "o/r",
		AddTopics:    []string{"rest"},
		RemoveTopics: []string{"cli"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	names, _ := got["names"].([]any)
	want := []string{"go", "rest"}
	if len(names) != len(want) {
		t.Fatalf("names: got %v want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d]: got %v want %s", i, n, want[i])
		}
	}
}

// TestEditRemoveTopicNotPresentWarns pins H28: removing a topic that
// wasn't on the repo used to exit 0 with the success marker; the user
// had no signal their typo was a no-op. We now emit a stderr note
// before the PUT, then complete the (idempotent) request.
func TestEditRemoveTopicNotPresentWarns(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{
		Name: "r", Topics: []string{"go"},
	})
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/topics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.TopicsPayload{Names: []string{"go"}})
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		RepoArg:      "o/r",
		RemoveTopics: []string{"nonexistent-topic"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stderr := tf.ErrOut.String()
	if !strings.Contains(stderr, "nonexistent-topic") {
		t.Errorf("stderr should warn about missing topic; got: %s", stderr)
	}
	if !strings.Contains(stderr, "not present") {
		t.Errorf("stderr should say 'not present'; got: %s", stderr)
	}
}

// TestEditRemoveTopicEmptyRepoStillWarns pins audit-I40: the H28
// reproducer reframed — when the repo has *zero* topics (server
// returns null/[]), `--remove-topic "FOO"` should still print the
// "not present" note. Pre-H28 the path was silent regardless of
// whether the topic was present-but-different or just missing; the
// audit re-flagged the empty-list case because the user expectation
// is the same: "I asked to remove X, X wasn't there, tell me."
func TestEditRemoveTopicEmptyRepoStillWarns(t *testing.T) {
	tf := cmdutiltest.New(t)
	// Server reports zero topics — the auditor's reproducer was a
	// fresh repo with no topics at all.
	tf.Server.RegisterJSON(http.MethodGet, "/api/v1/repos/o/r", 200, repos.Repo{
		Name: "r", Topics: nil,
	})
	tf.Server.Handle(http.MethodPut, "/api/v1/repos/o/r/topics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(repos.TopicsPayload{Names: []string{}})
	})

	opts := &options{
		IO:           tf.IOStreams,
		HTTPClient:   tf.Factory.HTTPClient,
		DefaultHost:  tf.Factory.DefaultHost,
		RepoArg:      "o/r",
		RemoveTopics: []string{"FOO"},
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	stderr := tf.ErrOut.String()
	if !strings.Contains(stderr, "FOO") || !strings.Contains(stderr, "not present") {
		t.Errorf("empty-repo --remove-topic should still warn; got stderr: %s", stderr)
	}
}

func TestEditNoFlagsErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		HTTPClient:  tf.Factory.HTTPClient,
		DefaultHost: tf.Factory.DefaultHost,
		RepoArg:     "o/r",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when no edit flags set")
	}
}

// TestEditRejectsEmptyTopic pins H27: an empty / whitespace topic
// value (`--add-topic ""`, `--remove-topic " "`) used to slip past
// the "nothing to do" guard and ship an empty topic through
// ReplaceTopics. Each topic flag now refuses a blank value with a
// flag-specific error.
func TestEditRejectsEmptyTopic(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts func(*options)
		want string
	}{
		{"add empty", func(o *options) { o.AddTopics = []string{""} }, "--add-topic"},
		{"remove blank", func(o *options) { o.RemoveTopics = []string{"  "} }, "--remove-topic"},
		{"replace empty", func(o *options) { o.Topics = []string{""} }, "--topic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tf := cmdutiltest.New(t)
			opts := &options{
				IO:          tf.IOStreams,
				HTTPClient:  tf.Factory.HTTPClient,
				DefaultHost: tf.Factory.DefaultHost,
				RepoArg:     "o/r",
			}
			tc.opts(opts)
			err := Run(context.Background(), opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q missing %q", err.Error(), tc.want)
			}
		})
	}
}

func TestMergeTopicsCaseInsensitive(t *testing.T) {
	got := mergeTopics([]string{"Go", "Cli"}, []string{"REST"}, []string{"cli"})
	want := []string{"Go", "REST"}
	if len(got) != len(want) {
		t.Fatalf("len: got %v want %v", got, want)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("[%d]: got %q want %q", i, v, want[i])
		}
	}
}

// TestMergeTopicsDuplicateAddIsIdempotent pins I54: `--add-topic foo`
// twice in the same invocation, or `--add-topic foo` when foo is
// already on the repo, must result in `[..., foo]` once — never
// `[..., foo, foo]`. mergeTopics's `seen` set is the source of truth;
// this test guards against a future regression that drops it.
func TestMergeTopicsDuplicateAddIsIdempotent(t *testing.T) {
	cases := []struct {
		name             string
		existing, add    []string
		wantContainsOnce string
	}{
		{"add twice in one call", nil, []string{"foo", "foo"}, "foo"},
		{"add existing topic", []string{"foo"}, []string{"foo"}, "foo"},
		{"add existing case-shift", []string{"Foo"}, []string{"FOO"}, "Foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeTopics(tc.existing, tc.add, nil)
			count := 0
			for _, v := range got {
				if strings.EqualFold(v, tc.wantContainsOnce) {
					count++
				}
			}
			if count != 1 {
				t.Errorf("topic %q appeared %d times in %v; want exactly 1", tc.wantContainsOnce, count, got)
			}
		})
	}
}
