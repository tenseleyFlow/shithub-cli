// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import "strings"

// ParseLinkHeader parses an RFC 8288 / 5988 Link header into a {rel: url}
// map. Pagination uses rel="next" / "prev" / "first" / "last". Multiple
// links in the same header are separated by commas; rel values may be
// quoted or bare. We honor the common forms gh and GitHub emit;
// pathological inputs (escaped quotes inside rel values, etc.) are not
// supported.
//
// Empty input returns an empty map. Unparseable entries are skipped
// silently — partial recovery is preferable to losing a "next" link
// because the "last" link had odd quoting.
func ParseLinkHeader(header string) map[string]string {
	out := map[string]string{}
	if header == "" {
		return out
	}
	for _, entry := range splitLinkEntries(header) {
		url, rel, ok := parseLinkEntry(entry)
		if !ok {
			continue
		}
		out[rel] = url
	}
	return out
}

// splitLinkEntries divides the comma-separated entries while respecting
// commas inside angle brackets (URLs may legally contain commas).
func splitLinkEntries(header string) []string {
	var (
		entries []string
		buf     strings.Builder
		depth   int
	)
	for _, r := range header {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				entries = append(entries, strings.TrimSpace(buf.String()))
				buf.Reset()
				continue
			}
		}
		buf.WriteRune(r)
	}
	if buf.Len() > 0 {
		entries = append(entries, strings.TrimSpace(buf.String()))
	}
	return entries
}

// parseLinkEntry pulls the URL and rel value out of a single entry of
// the form `<URL>; rel="value"; otherparam=...`. Returns ok=false when
// the entry is malformed.
func parseLinkEntry(entry string) (url, rel string, ok bool) {
	entry = strings.TrimSpace(entry)
	if !strings.HasPrefix(entry, "<") {
		return "", "", false
	}
	end := strings.Index(entry, ">")
	if end < 0 {
		return "", "", false
	}
	url = entry[1:end]

	remainder := entry[end+1:]
	for _, raw := range strings.Split(remainder, ";") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		eq := strings.Index(raw, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(raw[:eq])
		val := strings.TrimSpace(raw[eq+1:])
		val = strings.Trim(val, `"`)
		if strings.EqualFold(key, "rel") {
			rel = val
		}
	}
	if rel == "" {
		return "", "", false
	}
	return url, rel, true
}
