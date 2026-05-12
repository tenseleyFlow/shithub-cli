// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"fmt"
	"strings"
)

// Qualifier represents one `key:value` pair appended to the user's
// full-text query. Use ComposeQuery to flatten a list back into the
// final string.
type Qualifier struct {
	Key   string
	Value string
}

// ComposeQuery joins the user's free-text portion with the typed
// qualifiers. Values containing spaces / colons are quoted; empty
// qualifiers are skipped so callers can build the list with optional
// flags without filtering twice.
func ComposeQuery(text string, qs ...Qualifier) string {
	parts := make([]string, 0, len(qs)+1)
	if s := strings.TrimSpace(text); s != "" {
		parts = append(parts, s)
	}
	for _, q := range qs {
		if q.Key == "" || q.Value == "" {
			continue
		}
		parts = append(parts, formatQualifier(q.Key, q.Value))
	}
	return strings.Join(parts, " ")
}

// QualifierAll yields one Qualifier per value, all sharing the same
// key. Useful for repeatable flags like `--topic` / `--label`.
func QualifierAll(key string, values []string) []Qualifier {
	out := make([]Qualifier, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		out = append(out, Qualifier{Key: key, Value: v})
	}
	return out
}

// BoolQualifier converts a `--archived[=bool]`-style flag (a *bool that
// is nil when unset) into the matching qualifier. true → "true",
// false → "false", nil → empty.
func BoolQualifier(key string, b *bool) Qualifier {
	if b == nil {
		return Qualifier{}
	}
	if *b {
		return Qualifier{Key: key, Value: "true"}
	}
	return Qualifier{Key: key, Value: "false"}
}

// formatQualifier quotes the value when needed so the server's tokenizer
// keeps the right boundaries. We only quote on whitespace — gh treats
// `label:"good first issue"` as one qualifier and that's the form
// callers paste from issue trackers.
func formatQualifier(k, v string) string {
	if strings.ContainsAny(v, " \t") {
		return fmt.Sprintf(`%s:"%s"`, k, v)
	}
	return fmt.Sprintf("%s:%s", k, v)
}
