// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

// newOpts builds an Options seeded from the test factory.
func newOpts(t *testing.T) (*Options, *cmdutiltest.Factory) {
	t.Helper()
	tf := cmdutiltest.New(t)
	return &Options{
		IO:         tf.IOStreams,
		HTTPClient: tf.Factory.HTTPClient,
	}, tf
}

func TestRunGetDecodesBody(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "user"

	tf.Server.RegisterJSON("GET", "/api/v1/user", 200, map[string]any{
		"id":       1,
		"username": "mf",
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	out := tf.Out.String()
	if !strings.Contains(out, `"username":"mf"`) {
		t.Errorf("missing payload in output: %q", out)
	}
}

func TestRunPostWithFieldsBuildsJSON(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "repos/{owner}/{repo}/issues"
	opts.RepoFlag = "mf/cli"
	opts.Fields = []string{"title=Hello", "labels[]=bug", "labels[]=ux"}

	tf.Server.Handle("POST", "/api/v1/repos/mf/cli/issues", func(w http.ResponseWriter, r *http.Request) {
		// Verify method auto-flipped to POST and JSON body shape.
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type: got %q", r.Header.Get("Content-Type"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["title"] != "Hello" {
			t.Errorf("title: got %v", body["title"])
		}
		labels, _ := body["labels"].([]any)
		if len(labels) != 2 || labels[0] != "bug" || labels[1] != "ux" {
			t.Errorf("labels: got %v", labels)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":42}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunPlaceholderMissingErrors(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Endpoint = "repos/{owner}/{repo}/issues"
	// No -R, no SHITHUB_REPO, and cwd must not be a git working tree —
	// chdir into a fresh tempdir so resolvePlaceholders' git-remote
	// fallback finds nothing. Without this, the test runner's own repo
	// remote would silently fill {owner}/{repo}.
	chdirToTempDir(t)
	t.Setenv(EnvRepo, "")

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for unresolved placeholder")
	}
	if !strings.Contains(err.Error(), "{owner}") {
		t.Errorf("error should name the placeholder, got: %v", err)
	}
}

// chdirToTempDir points the process cwd at a fresh tempdir for the
// duration of the test, restoring on cleanup. Tests that depend on
// "no git context" rely on this.
func chdirToTempDir(t *testing.T) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestRunJQFilter(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "items"
	opts.JQ = ".[].id"

	tf.Server.RegisterJSON("GET", "/api/v1/items", 200, []any{
		map[string]any{"id": 1, "title": "a"},
		map[string]any{"id": 2, "title": "b"},
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := tf.Out.String(); got != "1\n2\n" {
		t.Errorf("jq output: got %q", got)
	}
}

func TestRunTemplateExecution(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "items"
	opts.Template = `{{range .}}{{.id}}:{{.title}}{{"\n"}}{{end}}`

	tf.Server.RegisterJSON("GET", "/api/v1/items", 200, []any{
		map[string]any{"id": 1, "title": "a"},
		map[string]any{"id": 2, "title": "b"},
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := tf.Out.String(); got != "1:a\n2:b\n" {
		t.Errorf("template output: got %q", got)
	}
}

// TestRunTemplateRejectsMissingKey covers C13: missing keys must
// surface as a template-execution error, never as the literal string
// "<no value>". Scripts piping `api -t '{{.field}}'` into `read` were
// silently consuming the marker.
func TestRunTemplateRejectsMissingKey(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "u"
	opts.Template = `{{.totally_missing_field}}`
	tf.Server.RegisterJSON("GET", "/api/v1/u", 200, map[string]any{"id": 1})

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error on missing template field")
	}
	if strings.Contains(tf.Out.String(), "<no value>") {
		t.Errorf("C13 regression: output contains '<no value>': %q", tf.Out.String())
	}
}

// TestRunGETWithFieldsBecomesQueryString covers C12: -f/-F on GET
// emits URL query params, not a body. Pre-D3b the server got a body
// on GET and responded with a confusing parse error.
func TestRunGETWithFieldsBecomesQueryString(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "search"
	opts.Method = "GET"
	opts.Fields = []string{"q=hello", "sort=stars"}
	var seenQuery string
	tf.Server.Handle("GET", "/api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		seenQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Order-insensitive substring check: both params must appear.
	for _, want := range []string{"q=hello", "sort=stars"} {
		if !strings.Contains(seenQuery, want) {
			t.Errorf("query missing %q; got: %s", want, seenQuery)
		}
	}
}

// TestRunPaginateRejectsNonArrayResponse covers C11: --paginate
// against an endpoint returning a single object must error, not
// silently wrap the object in a one-element array.
func TestRunPaginateRejectsNonArrayResponse(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "user"
	opts.Paginate = true
	tf.Server.Handle("GET", "/api/v1/user", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"login":"mf"}`))
	})

	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for --paginate on object endpoint")
	}
	if !strings.Contains(err.Error(), "JSON array") {
		t.Errorf("error should mention JSON array; got: %v", err)
	}
}

func TestRunMutexJQAndTemplate(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Endpoint = "u"
	opts.JQ = "."
	opts.Template = "{{.}}"
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected mutex error")
	}
}

func TestRunInputBodyForwarded(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "repos/{owner}/{repo}/issues"
	opts.RepoFlag = "mf/cli"
	opts.Input = "-"
	tf.In.WriteString(`{"title":"from stdin"}`)

	tf.Server.Handle("POST", "/api/v1/repos/mf/cli/issues", func(w http.ResponseWriter, r *http.Request) {
		body, _ := decodeRequest(r)
		if body["title"] != "from stdin" {
			t.Errorf("body title: got %v", body["title"])
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunHeadersForwarded(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "u"
	opts.RawHeaders = []string{"X-Custom: hello", "X-Multi: a"}

	tf.Server.Handle("GET", "/api/v1/u", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "hello" {
			t.Errorf("X-Custom: got %q", r.Header.Get("X-Custom"))
		}
		if r.Header.Get("X-Multi") != "a" {
			t.Errorf("X-Multi: got %q", r.Header.Get("X-Multi"))
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunPreviewSetsAcceptHeader(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "u"
	opts.Preview = "feature"

	tf.Server.Handle("GET", "/api/v1/u", func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Accept")
		want := "application/vnd.shithub.feature-preview+json"
		if got != want {
			t.Errorf("Accept: want %q got %q", want, got)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunIncludeAndSilent(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "u"
	opts.IncludeHeaders = true
	opts.Silent = true

	tf.Server.RegisterJSON("GET", "/api/v1/u", 200, map[string]any{"x": 1})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "HTTP/1.1 200") {
		t.Errorf("missing status line: %q", out)
	}
	if strings.Contains(out, `"x":`) {
		t.Errorf("Silent should suppress body, got %q", out)
	}
}

func TestRunCacheRoundTrip(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "u"
	opts.Cache = "1h"

	hits := 0
	tf.Server.Handle("GET", "/api/v1/u", func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"cached":true}`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected single network hit (second cached), got %d", hits)
	}
}

// TestRunCacheWarnsOnNonGet covers audit #150: pasting --cache onto a
// non-GET request previously dropped the cache silently. We now emit
// a stderr warning so the user knows their flag wasn't effective.
func TestRunCacheWarnsOnNonGet(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "repos"
	opts.Method = http.MethodPost
	opts.Cache = "1h"
	opts.RawFields = []string{"name=hello"}

	tf.Server.RegisterJSON(http.MethodPost, "/api/v1/repos", 201, map[string]any{"ok": true})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "--cache only applies to GET") {
		t.Errorf("expected non-GET cache warning; got %q", tf.ErrOut.String())
	}
}

func TestRunSlurpRequiresPaginate(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Endpoint = "u"
	opts.Slurp = true

	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error for --slurp without --paginate")
	}
}

func TestRunPaginateConcatArray(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "items"
	opts.Paginate = true

	tf.Server.Handle("GET", "/api/v1/items", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "page=2" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`[{"id":3}]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/items?page=2>; rel="next"`, tf.Server.URL()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := strings.TrimSpace(tf.Out.String())
	// concat → flat array of three elements
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 elements after concat, got %d (%v)", len(got), got)
	}
}

func TestRunPaginateSlurpWrapsPages(t *testing.T) {
	opts, tf := newOpts(t)
	opts.Endpoint = "items"
	opts.Paginate = true
	opts.Slurp = true

	tf.Server.Handle("GET", "/api/v1/items", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery == "page=2" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`[{"id":3}]`))
			return
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/items?page=2>; rel="next"`, tf.Server.URL()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
	})

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := strings.TrimSpace(tf.Out.String())
	var pages [][]map[string]any
	if err := json.Unmarshal([]byte(out), &pages); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if len(pages) != 2 || len(pages[0]) != 2 || len(pages[1]) != 1 {
		t.Errorf("--slurp shape: got %v", pages)
	}
}

func TestRunBadCacheDurationRejected(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Endpoint = "u"
	opts.Cache = "notaduration"
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error for bad --cache value")
	}
}

func TestRunBadInputFlagWithFields(t *testing.T) {
	opts, _ := newOpts(t)
	opts.Endpoint = "u"
	opts.Input = "-"
	opts.Fields = []string{"k=v"}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected --input vs -F mutex error")
	}
}

// decodeRequest is a small helper for handler tests that need to read
// the body without manually wrangling json.NewDecoder.
func decodeRequest(r *http.Request) (map[string]any, error) {
	var body map[string]any
	err := json.NewDecoder(r.Body).Decode(&body)
	return body, err
}
