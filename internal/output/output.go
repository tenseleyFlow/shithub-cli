// SPDX-License-Identifier: AGPL-3.0-or-later

// Package output owns the canonical --json / --jq / --template pipeline.
// Commands describe their machine-readable shape via Exporter, register
// the standard flags via AddFlags, and call Export to emit. The three
// flags are mutually exclusive with each other; emitting machine-readable
// output also disables the pager (handled here so command code doesn't
// have to remember).
package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// FlagNames documents the canonical flag spellings. Centralized so a
// future rename touches one place. Names match gh for muscle-memory parity.
const (
	FlagJSON      = "json"
	FlagJQ        = "jq"
	FlagTemplate  = "template"
	FlagJQShort   = "q"
	FlagTmplShort = "t"
)

// ErrFlagConflict is returned by Export when more than one of
// --json/--jq/--template is in effect. Surfaced to commands so they can
// translate to a user-friendly error with the right verb in the message.
var ErrFlagConflict = errors.New("--json, --jq, and --template are mutually exclusive")

// Exporter describes a command's machine-readable shape: the field names
// the user can project on, plus a Filter that maps full domain objects
// onto a JSON-marshalable subset. Filter is called once per Export call.
//
// Typical implementation lives next to the command's domain type:
//
//	type issueExporter struct{ fields []string }
//	func (e issueExporter) Fields() []string { return issueExportableFields }
//	func (e issueExporter) Filter(v any) (any, error) { ... }
type Exporter interface {
	// Fields lists every legal value the user can pass to --json.
	// Returned in the order the listing should be presented to the user.
	Fields() []string
	// Filter is given the raw domain object (whatever the command builds
	// up internally) and returns a value safe to encoding/json.Marshal.
	// The implementation projects to the user-requested field subset.
	Filter(v any) (any, error)
}

// Options captures the user's choice of output modality. Populated by the
// Cobra flag bindings AddFlags installs; passed to Export.
type Options struct {
	// JSONFields is the comma-separated value of --json. Empty when not set.
	// The sentinel "" + JSONSet=true means "list fields and exit" (gh shape).
	JSONFields string
	// JSONSet is true when --json was passed on the command line, regardless
	// of value. Distinguishes `--json` (list fields) from absence.
	JSONSet bool

	// JQ is the gojq filter expression. Empty when --jq is not set.
	JQ string

	// Template is the text/template body. Empty when --template is not set.
	Template string
}

// AddFlags wires the canonical flags onto cmd. Call from the command's
// builder; the populated Options is read by Export.
//
// `--json` takes a comma-separated field list as its value
// (`--json id,title` or `--json=id,title`). Field discovery uses the
// explicit-empty form `--json=` (Export lists fields and exits 0). The
// truly-bare `--json` (no `=`) is NOT supported here, intentionally:
// enabling it would require setting pflag's NoOptDefVal sentinel, which
// would then steal the next positional argument when users write
// `--json id,title` — breaking the daily-driver form. The audit #139
// decision (2026-05-12) prioritizes the space-separated form; see also
// the C02 spec for the formal rationale.
func AddFlags(cmd *cobra.Command, opts *Options) {
	cmd.Flags().StringVar(&opts.JSONFields, FlagJSON, "", "output JSON with the specified fields (comma-separated)")
	cmd.Flags().StringVarP(&opts.JQ, FlagJQ, FlagJQShort, "", "filter JSON output with a jq expression")
	cmd.Flags().StringVarP(&opts.Template, FlagTemplate, FlagTmplShort, "", "format JSON output with a Go template")

	// Mirror cobra's parse state into Options.JSONSet so command code can
	// distinguish "user asked for JSON" from "default empty value".
	preRun := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		opts.JSONSet = c.Flags().Changed(FlagJSON)
		if preRun != nil {
			return preRun(c, args)
		}
		return nil
	}
}

// Active reports whether any machine-readable mode is in effect. Useful
// for commands that switch off TUI niceties (spinners, pager) when
// emitting structured output.
func (o Options) Active() bool {
	return o.JSONSet || o.JQ != "" || o.Template != ""
}

// MarkWebMutuallyExclusive declares the standard output flags
// (--json, --jq, --template) incompatible with --web on cmd. Call
// from any builder that has both — without this, cobra silently lets
// both groups coexist and runtime picks --web, discarding the
// machine-readable request. The C-audit (C5, C15) flagged exactly
// this footgun.
//
// Three independent pair-wise calls (rather than one quadruple call)
// so cobra still allows the existing legal combinations among
// --json / --jq / --template.
func MarkWebMutuallyExclusive(cmd *cobra.Command) {
	cmd.MarkFlagsMutuallyExclusive("web", FlagJSON)
	cmd.MarkFlagsMutuallyExclusive("web", FlagJQ)
	cmd.MarkFlagsMutuallyExclusive("web", FlagTemplate)
}

// validate enforces mutual exclusion across --jq / --template against
// each other. --json may combine with --jq or --template because it
// drives the *projection*; jq/template format the projection.
func (o Options) validate() error {
	if o.JQ != "" && o.Template != "" {
		return ErrFlagConflict
	}
	return nil
}

// Export emits `data` to out per the options. The data is first passed
// through exporter.Filter (typically a field projection), then the result
// is JSON-marshaled, then optionally filtered through jq or templated
// through text/template. For TTY out and no jq/template, JSON is pretty
// printed; for non-TTY or filtered modes, JSON is compact.
//
// When --json is set with no value, the available fields are listed to
// out (one per line) and Export returns nil — matches gh's discoverability
// pattern.
func Export(out io.Writer, opts Options, exporter Exporter, data any, prettyJSON bool) error {
	if err := opts.validate(); err != nil {
		return err
	}

	// `--json` set with an empty value is treated as "list fields" — useful
	// for users who do `shithub repo list --json=` to discover field names.
	// Fields are emitted alphabetically (not in Exporter.Fields() insertion
	// order) so the listing is stable across exporter rewrites and matches
	// gh's `--json` discovery output. Audit #140 (2026-05-12) ratified this
	// behavior; the C02 spec line 59 says "gh behavior" which is also
	// alphabetical.
	if opts.JSONSet && opts.JSONFields == "" {
		fields := exporter.Fields()
		sorted := append([]string(nil), fields...)
		sort.Strings(sorted)
		for _, f := range sorted {
			if _, err := fmt.Fprintln(out, f); err != nil {
				return err
			}
		}
		return nil
	}

	// Field validation against the exporter's catalogue. Build the
	// requested set in one pass so the post-Filter projection can
	// reuse it without re-parsing the comma list.
	//
	// I8 (audit-I47 + I48): collect all unknown fields up front and
	// emit a single multi-line error with per-field suggestions where
	// available. Pre-fix the loop bailed on the first typo, forcing
	// users to fix one field at a time; and there was no Levenshtein
	// suggestion, so `--json closed` (where the field is `closedAt`)
	// got the bare "unknown JSON field" without a hint.
	var requestedSet map[string]struct{}
	if opts.JSONFields != "" {
		requested := strings.Split(opts.JSONFields, ",")
		requestedSet = make(map[string]struct{}, len(requested))
		validFields := exporter.Fields()
		valid := map[string]struct{}{}
		for _, f := range validFields {
			valid[f] = struct{}{}
		}
		var bad []string
		for _, r := range requested {
			r = strings.TrimSpace(r)
			// H25: pre-fix, `--json ",name"` produced `unknown JSON field ""`
			// — readable as "the empty string isn't a field", not as the
			// stray comma it actually is. Tell the user the input shape is
			// wrong before falling into the field-name catalogue.
			if r == "" {
				return fmt.Errorf("--json value contains an empty field (check for stray or trailing commas)")
			}
			if _, ok := valid[r]; !ok {
				bad = append(bad, r)
				continue
			}
			requestedSet[r] = struct{}{}
		}
		if len(bad) > 0 {
			return unknownJSONFieldsError(bad, validFields)
		}
	}

	filtered, err := exporter.Filter(data)
	if err != nil {
		return fmt.Errorf("output: filter: %w", err)
	}

	// G3 (F1): every exporter's Filter builds the full gh-compat field
	// map regardless of what the user asked for. Pre-fix, that map went
	// straight to JSON and the user got every field — `--json title`
	// was decorative. Project here so the listing actually filters.
	// Exporters that return something other than the standard
	// map/[]map shapes pass through unchanged (they're already
	// projecting themselves; the projection guard runs only when the
	// shape is recognized).
	if requestedSet != nil {
		filtered = projectFields(filtered, requestedSet)
	}

	encoded, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("output: marshal: %w", err)
	}

	switch {
	case opts.JQ != "":
		return applyJQ(out, opts.JQ, encoded)
	case opts.Template != "":
		return applyTemplate(out, opts.Template, encoded)
	default:
		return writeJSON(out, encoded, prettyJSON)
	}
}

// writeJSON either pretty-prints or compacts the bytes before writing.
// Pretty mode uses 2-space indent + trailing newline; compact mode uses
// no indent and a single trailing newline so callers can pipe into less.
func writeJSON(out io.Writer, encoded []byte, pretty bool) error {
	if pretty {
		var dst bytes.Buffer
		if err := json.Indent(&dst, encoded, "", "  "); err != nil {
			return err
		}
		dst.WriteByte('\n')
		_, err := out.Write(dst.Bytes())
		return err
	}
	_, err := out.Write(append(encoded, '\n'))
	return err
}

// applyJQ runs the user's gojq query against the encoded JSON and writes
// each emitted value on its own line. Strings are unquoted (matches `jq -r`
// behavior, which is what command-line filtering usually wants).
func applyJQ(out io.Writer, expr string, encoded []byte) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("output: parse jq: %w", err)
	}

	var input any
	if err := json.Unmarshal(encoded, &input); err != nil {
		return fmt.Errorf("output: re-decode for jq: %w", err)
	}

	iter := query.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("output: jq runtime: %w", err)
		}
		if err := writeJQValue(out, v); err != nil {
			return err
		}
	}
}

func writeJQValue(out io.Writer, v any) error {
	// Strings: unquoted, one per line. Matches `jq -r` for raw output.
	if s, ok := v.(string); ok {
		_, err := fmt.Fprintln(out, s)
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = out.Write(append(b, '\n'))
	return err
}

// applyTemplate compiles `tmpl` and executes it against the decoded JSON
// document. Helper funcs are pre-registered: `color`, `truncate`,
// `timeago`, `tablerow` — the same surface gh exposes.
//
// Color helpers are stubs that pass strings through unchanged: the
// command builder injects a real iostreams-aware FuncMap when relevant.
// We expose a hookless default here to keep the package independent of
// internal/iostreams (avoids a cycle: iostreams uses output, output
// would use iostreams).
func applyTemplate(out io.Writer, tmpl string, encoded []byte) error {
	t, err := template.New("output").Funcs(defaultFuncs()).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("output: parse template: %w", err)
	}

	var input any
	if err := json.Unmarshal(encoded, &input); err != nil {
		return fmt.Errorf("output: re-decode for template: %w", err)
	}

	// G12 (F37): mirror `shithub api -t` (A6) — buffer the rendered
	// template, write it, and ensure a trailing newline so the output
	// doesn't merge into the next shell prompt or pipeline reader's
	// line boundary. `--jq` already terminates with `\n`; without this
	// `repo view -t '{{.fullName}}'` was the lone outlier.
	var buf bytes.Buffer
	if err := t.Execute(&buf, input); err != nil {
		return fmt.Errorf("output: execute template: %w", err)
	}
	rendered := buf.Bytes()
	if _, err := out.Write(rendered); err != nil {
		return err
	}
	if len(rendered) == 0 || rendered[len(rendered)-1] != '\n' {
		_, err := io.WriteString(out, "\n")
		return err
	}
	return nil
}

// defaultFuncs returns the FuncMap available inside --template bodies.
// Implementations are conservative; for fancy color the command should
// inject overrides via cobra context (a future refinement).
func defaultFuncs() template.FuncMap {
	return template.FuncMap{
		"truncate": func(width int, s string) string {
			if width <= 0 || len(s) <= width {
				return s
			}
			return s[:width-1] + "…"
		},
		"timeago": func(v any) string {
			s, ok := v.(string)
			if !ok {
				return ""
			}
			t, err := time.Parse(time.RFC3339, s)
			if err != nil {
				return s
			}
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < time.Hour:
				return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
			case d < 24*time.Hour:
				return fmt.Sprintf("%d hours ago", int(d.Hours()))
			default:
				return fmt.Sprintf("%d days ago", int(d/(24*time.Hour)))
			}
		},
		"color": func(_, s string) string { return s }, // overridden by command
		"tablerow": func(cells ...any) string {
			parts := make([]string, len(cells))
			for i, c := range cells {
				parts[i] = fmt.Sprint(c)
			}
			return strings.Join(parts, "\t") + "\n"
		},
	}
}

// projectFields trims map keys that aren't in the requested set,
// recursing one level into slices so `[]map[string]any` (the listing
// shape every list exporter returns) gets projected per-element.
// Other shapes pass through unchanged — an exporter that returns a
// scalar or a slice of strings is already self-projecting.
func projectFields(v any, fields map[string]struct{}) any {
	switch t := v.(type) {
	case map[string]any:
		return projectMap(t, fields)
	case []map[string]any:
		out := make([]map[string]any, len(t))
		for i, m := range t {
			out[i] = projectMap(m, fields)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = projectFields(e, fields)
		}
		return out
	default:
		return v
	}
}

func projectMap(m map[string]any, fields map[string]struct{}) map[string]any {
	out := make(map[string]any, len(fields))
	for k := range fields {
		if val, ok := m[k]; ok {
			out[k] = val
		}
	}
	return out
}

// unknownJSONFieldsError builds the I48 multi-field error message:
// every bad field on its own line with an optional "did you mean ...?"
// suggestion, followed by the full valid catalog. Single-field path
// still produces a sensible one-liner.
func unknownJSONFieldsError(bad, validFields []string) error {
	if len(bad) == 1 {
		// Keep the single-field path single-line; older scripts may
		// scrape the message and a wrap would surprise them.
		want := bad[0]
		if s := suggestField(want, validFields); s != "" {
			return fmt.Errorf("unknown JSON field %q (did you mean %q?); valid: %s",
				want, s, strings.Join(validFields, ", "))
		}
		return fmt.Errorf("unknown JSON field %q; valid: %s",
			want, strings.Join(validFields, ", "))
	}
	var msg strings.Builder
	msg.WriteString("unknown JSON field(s):")
	for _, b := range bad {
		if s := suggestField(b, validFields); s != "" {
			fmt.Fprintf(&msg, "\n  - %q (did you mean %q?)", b, s)
		} else {
			fmt.Fprintf(&msg, "\n  - %q", b)
		}
	}
	fmt.Fprintf(&msg, "\n  valid: %s", strings.Join(validFields, ", "))
	return errors.New(msg.String())
}

// suggestField returns the closest match from `valid` to `want` within
// Levenshtein distance 2, or "" when no close match exists. Two-char
// edit budget catches transposes ("nuumber" → "number") and one
// missing/extra char ("boddy" → "body", "closed" → "closedAt") without
// catching unrelated fields. Empty if the closest match exceeds the
// budget; the caller falls back to the plain error message.
func suggestField(want string, valid []string) string {
	best := ""
	bestDist := 3 // 3 = "not close enough"; we accept ≤2
	for _, v := range valid {
		d := editDistance(want, v)
		if d < bestDist {
			best = v
			bestDist = d
		}
	}
	if bestDist > 2 {
		return ""
	}
	return best
}

// editDistance computes Levenshtein distance between a and b. The
// catalogs are small (≤30 fields, ≤25 chars each) so the O(len(a) *
// len(b)) DP is fine — no need for a fancy library.
func editDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = del
			if ins < curr[j] {
				curr[j] = ins
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}
