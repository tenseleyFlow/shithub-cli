// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFieldFlag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in               string
		wantKey, wantVal string
		expectError      bool
	}{
		{in: "k=v", wantKey: "k", wantVal: "v"},
		{in: "k=", wantKey: "k", wantVal: ""},
		{in: "k=v=more", wantKey: "k", wantVal: "v=more"},
		{in: "=value", expectError: true},
		{in: "novsep", expectError: true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			k, v, err := parseFieldFlag(tc.in)
			if tc.expectError {
				if err == nil {
					t.Errorf("want error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if k != tc.wantKey {
				t.Errorf("key: want %q got %q", tc.wantKey, k)
			}
			if v != tc.wantVal {
				t.Errorf("value: want %q got %q", tc.wantVal, v)
			}
		})
	}
}

func TestResolveFieldValueTypePromotion(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   fieldEntry
		want any
	}{
		{name: "bool true", in: fieldEntry{value: "true"}, want: true},
		{name: "bool false", in: fieldEntry{value: "false"}, want: false},
		{name: "null", in: fieldEntry{value: "null"}, want: nil},
		{name: "int", in: fieldEntry{value: "42"}, want: int64(42)},
		{name: "float", in: fieldEntry{value: "3.14"}, want: 3.14},
		{name: "string", in: fieldEntry{value: "hello"}, want: "hello"},
		{name: "string-looking-bool with raw", in: fieldEntry{value: "true", raw: true}, want: "true"},
		{name: "raw string", in: fieldEntry{value: "42", raw: true}, want: "42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFieldValue(tc.in, &bytes.Buffer{})
			if err != nil {
				t.Fatalf("resolveFieldValue: %v", err)
			}
			if got != tc.want {
				t.Errorf("want %v (%T) got %v (%T)", tc.want, tc.want, got, got)
			}
		})
	}
}

func TestResolveFieldValueAtFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("hello from file\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	got, err := resolveFieldValue(fieldEntry{value: "@" + path}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("resolveFieldValue: %v", err)
	}
	if got != "hello from file\n" {
		t.Errorf("file contents: got %q", got)
	}
}

func TestResolveFieldValueAtDash(t *testing.T) {
	t.Parallel()
	stdin := bytes.NewBufferString("stdin payload")
	got, err := resolveFieldValue(fieldEntry{value: "@-"}, stdin)
	if err != nil {
		t.Fatalf("resolveFieldValue: %v", err)
	}
	if got != "stdin payload" {
		t.Errorf("stdin: got %q", got)
	}
}

func TestBuildJSONBodySingleScalar(t *testing.T) {
	t.Parallel()
	entries, err := fieldEntriesFromFlags([]string{"title=Hello", "draft=true"}, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body, err := buildJSONBody(entries, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if got["title"] != "Hello" {
		t.Errorf("title: got %v", got["title"])
	}
	if got["draft"] != true {
		t.Errorf("draft: got %v", got["draft"])
	}
}

func TestBuildJSONBodyArrayKey(t *testing.T) {
	t.Parallel()
	entries, err := fieldEntriesFromFlags([]string{"labels[]=bug", "labels[]=ux"}, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body, err := buildJSONBody(entries, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var got map[string][]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if len(got["labels"]) != 2 || got["labels"][0] != "bug" || got["labels"][1] != "ux" {
		t.Errorf("labels: got %v", got["labels"])
	}
}

func TestBuildJSONBodyOrderStable(t *testing.T) {
	t.Parallel()
	entries, err := fieldEntriesFromFlags([]string{"z=1", "a=2", "m=3"}, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body, err := buildJSONBody(entries, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// We hand-rolled the object specifically so insertion order is the
	// emit order. Verify by string-position checks.
	s := string(body)
	zIdx := strings.Index(s, `"z"`)
	aIdx := strings.Index(s, `"a"`)
	mIdx := strings.Index(s, `"m"`)
	if zIdx >= aIdx || aIdx >= mIdx {
		t.Errorf("emit order should follow flag order, got %s", s)
	}
}

func TestBuildJSONBodyEmpty(t *testing.T) {
	t.Parallel()
	body, err := buildJSONBody(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if body != nil {
		t.Errorf("empty entries should yield nil, got %q", body)
	}
}
