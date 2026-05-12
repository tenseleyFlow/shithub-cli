// SPDX-License-Identifier: AGPL-3.0-or-later

// Package shared holds helpers reused across `shithub label`
// subcommands. Today: color validation and three-digit hex expansion.
package shared

import (
	"fmt"
	"strings"
)

// NormalizeColor accepts a 3- or 6-digit hex string (with or without a
// leading '#') and returns the canonical 6-digit lowercase form
// shithub stores. Other inputs return an error.
//
// Examples:
//
//	"abc"     → "aabbcc"
//	"#abc"    → "aabbcc"
//	"AbCdEf"  → "abcdef"
//	"red"     → error
func NormalizeColor(in string) (string, error) {
	s := strings.TrimSpace(in)
	s = strings.TrimPrefix(s, "#")
	if !allHex(s) {
		return "", fmt.Errorf("color: %q is not hex", in)
	}
	switch len(s) {
	case 3:
		// Expand "abc" → "aabbcc". Mirrors the CSS shorthand rule and
		// matches GitHub's lenient input handling.
		return strings.ToLower(fmt.Sprintf("%c%c%c%c%c%c", s[0], s[0], s[1], s[1], s[2], s[2])), nil
	case 6:
		return strings.ToLower(s), nil
	default:
		return "", fmt.Errorf("color: %q must be 3 or 6 hex digits", in)
	}
}

// allHex reports whether every byte is 0-9 / a-f / A-F.
func allHex(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
