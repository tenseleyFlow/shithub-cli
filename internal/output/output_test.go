// SPDX-License-Identifier: AGPL-3.0-or-later

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// TestMarkWebMutuallyExclusive: C-audit C5/C15. When a command has
// both --web and the standard output flags, supplying both must be
// rejected at parse time. Three pair-wise groups means --json + --jq
// stays legal (existing legal combination).
func TestMarkWebMutuallyExclusive(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"web alone", []string{"--web"}, false},
		{"json alone", []string{"--json", "id"}, false},
		{"jq alone", []string{"--jq", ".id"}, false},
		{"template alone", []string{"--template", "{{.id}}"}, false},
		{"json+jq still legal", []string{"--json", "id", "--jq", ".id"}, false},
		{"web + json rejected", []string{"--web", "--json", "id"}, true},
		{"web + jq rejected", []string{"--web", "--jq", ".id"}, true},
		{"web + template rejected", []string{"--web", "--template", "{{.id}}"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newTestCmdWithWebAndOutput()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Errorf("err: got %v, wantErr=%v", err, tc.wantErr)
			}
			// cobra's group-message form: "if any flags in the group
			// [a b] are set none of the others can be; [...] were all set"
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), "none of the others can be") {
				t.Errorf("error should be cobra's mutex form; got: %v", err)
			}
		})
	}
}

// fullExporter mimics every command's real exporter: Filter ignores
// the request and always builds a map of every field it knows about.
// Pre-G3 Export emitted that whole map regardless of --json. The
// projection tests rely on this no-op Filter so they can pin Export
// itself as the projection site.
type fullExporter struct {
	fields []string
}

func (f fullExporter) Fields() []string { return f.fields }
func (f fullExporter) Filter(v any) (any, error) {
	// Echo back the value verbatim — it already carries every field.
	return v, nil
}

// G3 (F1): --json projection must trim Filter output to the requested
// field set. Pre-fix Export validated the field list and emitted the
// full map anyway — `--json id` was decorative. Pins the contract on
// both single-record (map) and listing ([]map) shapes.
func TestExportProjectsToRequestedFields_SingleMap(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fullExporter{fields: []string{"id", "title", "body", "url"}}
	data := map[string]any{
		"id": 7, "title": "hello", "body": "long form", "url": "https://x",
	}
	opts := Options{JSONFields: "id,title", JSONSet: true}
	if err := Export(&buf, opts, exp, data, false); err != nil {
		t.Fatalf("Export: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 keys, got %d: %v", len(got), got)
	}
	if _, ok := got["body"]; ok {
		t.Errorf("body should be projected out: %v", got)
	}
	if _, ok := got["url"]; ok {
		t.Errorf("url should be projected out: %v", got)
	}
}

func TestExportProjectsToRequestedFields_ListOfMaps(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fullExporter{fields: []string{"id", "title", "body"}}
	data := []map[string]any{
		{"id": 1, "title": "a", "body": "aa"},
		{"id": 2, "title": "b", "body": "bb"},
	}
	opts := Options{JSONFields: "title", JSONSet: true}
	if err := Export(&buf, opts, exp, data, false); err != nil {
		t.Fatalf("Export: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}
	for i, row := range got {
		if len(row) != 1 {
			t.Errorf("row[%d] should have 1 key, got %d: %v", i, len(row), row)
		}
		if _, ok := row["title"]; !ok {
			t.Errorf("row[%d] missing title: %v", i, row)
		}
	}
}

// Projection respects requested ordering and is robust to whitespace
// in the comma list (`--json id, title , body` is still legal).
func TestExportProjectsHandlesWhitespace(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fullExporter{fields: []string{"id", "title", "body"}}
	data := map[string]any{"id": 1, "title": "a", "body": "z"}
	opts := Options{JSONFields: " id , body ", JSONSet: true}
	if err := Export(&buf, opts, exp, data, false); err != nil {
		t.Fatalf("Export: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(buf.Bytes(), &got)
	if len(got) != 2 {
		t.Errorf("want 2 keys, got %d: %v", len(got), got)
	}
	if _, ok := got["title"]; ok {
		t.Errorf("title should be projected out (only id, body requested): %v", got)
	}
}

// Projection is skipped when no --json fields were requested (e.g.
// the --jq path still needs the full document to filter against).
func TestExportSkipsProjectionWithoutJSONFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	exp := fullExporter{fields: []string{"id", "title"}}
	data := map[string]any{"id": 1, "title": "a"}
	opts := Options{JQ: ".id"}
	if err := Export(&buf, opts, exp, data, false); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "1" {
		t.Errorf("jq output: got %q want %q", got, "1")
	}
}

// newTestCmdWithWebAndOutput builds a no-op cobra command that has
// both `--web` and the output flag set, with the mutex applied.
func newTestCmdWithWebAndOutput() *cobra.Command {
	var web bool
	opts := &Options{}
	cmd := &cobra.Command{
		Use:  "x",
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	cmd.Flags().BoolVar(&web, "web", false, "")
	AddFlags(cmd, opts)
	MarkWebMutuallyExclusive(cmd)
	return cmd
}

// TestApplyTemplateAppendsTrailingNewline pins F37: every command's
// `-t '{{...}}'` template output must end in `\n` (matching `shithub
// api -t`'s A6 behavior). Pre-fix `repo view -t '{{.fullName}}'`
// emitted the value without a newline, merging into the next shell
// prompt or pipeline reader's line boundary.
func TestApplyTemplateAppendsTrailingNewline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tmpl string
		in   string
		want string
	}{
		{"no trailing newline", `{{.fullName}}`, `{"fullName":"octo/hello"}`, "octo/hello\n"},
		{"already ends in newline", "{{.fullName}}\n", `{"fullName":"octo/hello"}`, "octo/hello\n"},
		{"empty template", ``, `{"x":1}`, "\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := applyTemplate(&buf, tc.tmpl, []byte(tc.in)); err != nil {
				t.Fatalf("applyTemplate: %v", err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
