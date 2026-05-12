// SPDX-License-Identifier: AGPL-3.0-or-later

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// fakeExporter projects the supplied data through json.Marshal-friendly
// shape; the Filter is the identity since we already pass in plain maps
// in tests. Fields lists what's "available" for --json field validation.
type fakeExporter struct {
	fields []string
}

func (f fakeExporter) Fields() []string          { return f.fields }
func (f fakeExporter) Filter(v any) (any, error) { return v, nil }

func TestExportPrettyJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"id", "title"}}
	data := map[string]any{"id": 7, "title": "hello"}

	opts := Options{JSONFields: "id,title", JSONSet: true}
	if err := Export(&buf, opts, exp, data, true); err != nil {
		t.Fatalf("Export: %v", err)
	}

	if !strings.Contains(buf.String(), "\n  \"id\":") {
		t.Errorf("pretty JSON should have 2-space indent, got: %q", buf.String())
	}
}

func TestExportCompactJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"id"}}
	opts := Options{JSONFields: "id", JSONSet: true}
	if err := Export(&buf, opts, exp, map[string]any{"id": 1}, false); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if strings.Contains(buf.String(), "  ") {
		t.Errorf("compact JSON should have no indent, got: %q", buf.String())
	}
	// Sanity: still parseable.
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
}

func TestExportListsFieldsWhenJSONHasNoValue(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"title", "id", "body"}}
	opts := Options{JSONSet: true, JSONFields: ""}

	if err := Export(&buf, opts, exp, nil, true); err != nil {
		t.Fatalf("Export: %v", err)
	}
	// Fields listed alphabetically, one per line.
	want := "body\nid\ntitle\n"
	if got := buf.String(); got != want {
		t.Errorf("field listing: want %q got %q", want, got)
	}
}

func TestExportRejectsUnknownField(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"id"}}
	opts := Options{JSONFields: "id,bogus", JSONSet: true}

	err := Export(&buf, opts, exp, map[string]any{"id": 1}, false)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name the bad field: %v", err)
	}
}

func TestExportJQFilter(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"items"}}
	data := map[string]any{
		"items": []any{
			map[string]any{"id": 1, "title": "a"},
			map[string]any{"id": 2, "title": "b"},
		},
	}
	opts := Options{
		JSONFields: "items",
		JSONSet:    true,
		JQ:         ".items[].id",
	}

	if err := Export(&buf, opts, exp, data, false); err != nil {
		t.Fatalf("Export jq: %v", err)
	}
	if got := buf.String(); got != "1\n2\n" {
		t.Errorf("jq output: want %q got %q", "1\n2\n", got)
	}
}

func TestExportJQStringsUnquoted(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"name"}}
	opts := Options{JSONFields: "name", JSONSet: true, JQ: ".name"}
	if err := Export(&buf, opts, exp, map[string]any{"name": "hello"}, false); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := buf.String(); got != "hello\n" {
		t.Errorf("string output should be unquoted; got %q", got)
	}
}

func TestExportTemplate(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"items"}}
	data := map[string]any{
		"items": []any{
			map[string]any{"id": 1.0, "title": "a"},
			map[string]any{"id": 2.0, "title": "b"},
		},
	}
	opts := Options{
		JSONFields: "items",
		JSONSet:    true,
		Template:   `{{range .items}}{{.id}}:{{.title}}{{"\n"}}{{end}}`,
	}
	if err := Export(&buf, opts, exp, data, false); err != nil {
		t.Fatalf("Export template: %v", err)
	}
	want := "1:a\n2:b\n"
	if got := buf.String(); got != want {
		t.Errorf("template output: want %q got %q", want, got)
	}
}

func TestExportRejectsJQAndTemplateTogether(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fakeExporter{fields: []string{"x"}}
	opts := Options{JSONFields: "x", JSONSet: true, JQ: ".x", Template: "{{.x}}"}

	err := Export(&buf, opts, exp, map[string]any{"x": 1}, false)
	if err == nil {
		t.Fatal("expected ErrFlagConflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutual exclusion, got %v", err)
	}
}

func TestOptionsActive(t *testing.T) {
	t.Parallel()
	cases := []struct {
		opts Options
		want bool
	}{
		{Options{}, false},
		{Options{JSONSet: true}, true},
		{Options{JQ: ".x"}, true},
		{Options{Template: "{{.}}"}, true},
	}
	for i, tc := range cases {
		if got := tc.opts.Active(); got != tc.want {
			t.Errorf("[%d] Active: want %v got %v", i, tc.want, got)
		}
	}
}

func TestDefaultTemplateFuncs(t *testing.T) {
	t.Parallel()
	funcs := defaultFuncs()

	// truncate
	if got := funcs["truncate"].(func(int, string) string)(3, "abcdef"); got != "ab…" {
		t.Errorf("truncate: got %q", got)
	}
	if got := funcs["truncate"].(func(int, string) string)(10, "short"); got != "short" {
		t.Errorf("truncate no-op: got %q", got)
	}

	// timeago — non-string returns ""
	if got := funcs["timeago"].(func(any) string)(42); got != "" {
		t.Errorf("timeago non-string: want empty, got %q", got)
	}
	// unparseable string passes through
	if got := funcs["timeago"].(func(any) string)("not-rfc3339"); got != "not-rfc3339" {
		t.Errorf("timeago unparseable: got %q", got)
	}

	// color (stub)
	if got := funcs["color"].(func(string, string) string)("red", "x"); got != "x" {
		t.Errorf("color stub: got %q", got)
	}
}
