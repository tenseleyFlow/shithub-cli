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

	// Field validation against the exporter's catalogue.
	if opts.JSONFields != "" {
		requested := strings.Split(opts.JSONFields, ",")
		for i, r := range requested {
			requested[i] = strings.TrimSpace(r)
		}
		valid := map[string]struct{}{}
		for _, f := range exporter.Fields() {
			valid[f] = struct{}{}
		}
		for _, r := range requested {
			if _, ok := valid[r]; !ok {
				return fmt.Errorf("unknown JSON field %q; valid: %s", r, strings.Join(exporter.Fields(), ", "))
			}
		}
	}

	filtered, err := exporter.Filter(data)
	if err != nil {
		return fmt.Errorf("output: filter: %w", err)
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

	if err := t.Execute(out, input); err != nil {
		return fmt.Errorf("output: execute template: %w", err)
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
