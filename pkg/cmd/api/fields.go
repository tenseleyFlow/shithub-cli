// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// fieldEntry is one -F or -f flag parsed. The boolean `raw` records the
// user's intent: -F runs autodetection on the value (bool/number/null/
// @file), -f keeps the value as a string regardless.
type fieldEntry struct {
	key   string
	value string
	raw   bool
}

// parseFieldFlag splits a `key=value` string. Splits on the FIRST `=`
// because the value may itself contain `=` (template strings, encoded
// blobs). An empty key is rejected; an empty value is legal.
func parseFieldFlag(arg string) (key, value string, err error) {
	idx := strings.IndexByte(arg, '=')
	if idx <= 0 {
		return "", "", fmt.Errorf("api: field %q missing '=' separator", arg)
	}
	return arg[:idx], arg[idx+1:], nil
}

// queryStringFromFields encodes -f/-F flags as URL query parameters for
// the GET-with-fields case (C-audit C12). Values flow through the same
// expansion as the body path: -F runs bool/number/@file detection, -f
// keeps strings verbatim. Order is preserved for stable URL shape.
func queryStringFromFields(typed, rawFields []string, stdin io.Reader) (string, error) {
	entries, err := fieldEntriesFromFlags(typed, rawFields)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		val := e.value
		if e.raw {
			val = e.value
		} else {
			// Typed (-F) supports @file; expand it just like the body
			// path does so users don't have to switch to -f on GET.
			if strings.HasPrefix(e.value, "@") {
				data, ferr := readInput(strings.TrimPrefix(e.value, "@"), stdin)
				if ferr != nil {
					return "", ferr
				}
				val = string(data)
			}
		}
		parts = append(parts, url.QueryEscape(e.key)+"="+url.QueryEscape(val))
	}
	return strings.Join(parts, "&"), nil
}

// fieldEntriesFromFlags merges the -F and -f flag slices preserving the
// caller's order, which becomes the JSON object insertion order. Errors
// fail fast — one bad field aborts the whole call.
func fieldEntriesFromFlags(typed, rawFields []string) ([]fieldEntry, error) {
	entries := make([]fieldEntry, 0, len(typed)+len(rawFields))
	for _, f := range typed {
		k, v, err := parseFieldFlag(f)
		if err != nil {
			return nil, err
		}
		entries = append(entries, fieldEntry{key: k, value: v, raw: false})
	}
	for _, f := range rawFields {
		k, v, err := parseFieldFlag(f)
		if err != nil {
			return nil, err
		}
		entries = append(entries, fieldEntry{key: k, value: v, raw: true})
	}
	return entries, nil
}

// resolveFieldValue does the type-promotion dance for -F flags:
//
//	"true" / "false"      → bool
//	"null"                → nil
//	"@<path>"             → file contents (bytes, decoded as string)
//	parsable as int       → int64
//	parsable as float     → float64
//	otherwise             → string verbatim
//
// -f flags skip the promotion (value stays a string). The `stdin`
// reader is consulted when the file path is `-`, so callers can pipe
// JSON in via shell redirection.
func resolveFieldValue(e fieldEntry, stdin io.Reader) (any, error) {
	if e.raw {
		return e.value, nil
	}
	v := e.value
	switch {
	case v == "true":
		return true, nil
	case v == "false":
		return false, nil
	case v == "null":
		return nil, nil
	case strings.HasPrefix(v, "@"):
		return readFieldFile(v[1:], stdin)
	}
	if i, err := strconv.ParseInt(v, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f, nil
	}
	return v, nil
}

// readFieldFile loads the named file. Special-case "-" pulls from stdin
// (matches gh's syntax: `-F body=@-`). The decoded content is returned
// as a string for embedding into the outgoing JSON body; binary content
// works but produces an invalid UTF-8 string under the hood — fine for
// the JSON encoder, since it base64s invalid sequences.
func readFieldFile(path string, stdin io.Reader) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("api: read @- from stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is user-supplied; this is the api command's whole point
	if err != nil {
		return "", fmt.Errorf("api: read @%s: %w", path, err)
	}
	return string(data), nil
}

// buildJSONBody turns the parsed entries into a JSON object. Multiple
// fields with the same key collapse into an array (gh behavior: use
// `-F labels[]=bug -F labels[]=ux` to send an array field). Keys that
// end with `[]` are recognized as array-builders even when only one
// value is supplied.
//
// Returns the encoded bytes ready to put on the wire, or nil when
// `entries` is empty.
func buildJSONBody(entries []fieldEntry, stdin io.Reader) ([]byte, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	// Ordered insertion via a parallel slice — Go maps don't preserve
	// order, but stable key order is nice for snapshot tests + makes the
	// resulting JSON deterministic.
	keys := []string{}
	values := map[string][]any{}

	for _, e := range entries {
		val, err := resolveFieldValue(e, stdin)
		if err != nil {
			return nil, err
		}
		key := strings.TrimSuffix(e.key, "[]")
		isArray := strings.HasSuffix(e.key, "[]")

		if _, seen := values[key]; !seen {
			keys = append(keys, key)
		}
		if isArray {
			values[key] = append(values[key], val)
		} else {
			if existing, ok := values[key]; ok {
				// Repeated non-array key: implicit collapse into an array.
				values[key] = append(existing, val)
			} else {
				values[key] = []any{val}
			}
		}
	}

	// Emit the JSON. For keys that received exactly one non-array entry,
	// unwrap the array; otherwise serialize as an array.
	type kv struct {
		key string
		v   any
	}
	rendered := make([]kv, 0, len(keys))
	for _, k := range keys {
		vs := values[k]
		// Detect "always array" intent: any entry with k+"[]" appeared.
		// We don't track which entries were array-tagged at this layer;
		// the gh-compatible rule is "more than one value → array, single
		// value with [] suffix → single-element array". We approximate by
		// scanning entries again.
		alwaysArray := false
		for _, e := range entries {
			if strings.TrimSuffix(e.key, "[]") == k && strings.HasSuffix(e.key, "[]") {
				alwaysArray = true
				break
			}
		}
		if !alwaysArray && len(vs) == 1 {
			rendered = append(rendered, kv{key: k, v: vs[0]})
		} else {
			rendered = append(rendered, kv{key: k, v: vs})
		}
	}

	// json.Marshal on a map[string]any does not preserve insertion order;
	// hand-roll the object to keep -F flag order stable.
	var buf strings.Builder
	buf.WriteByte('{')
	for i, kv := range rendered {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := json.Marshal(kv.key)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(kv.v)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}
