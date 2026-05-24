// SPDX-License-Identifier: AGPL-3.0-or-later

package shared

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxTitleLen caps issue/PR titles. gh enforces 256 chars on the
// GitHub side and the human formatter truncates well before that; the
// limit is more about catching paste-of-an-entire-stacktrace mishaps
// than a real product constraint.
const MaxTitleLen = 256

// ValidateTitle normalizes + validates a title for `issue create`,
// `pr create`, and their respective `edit` paths.
//
// audit-I52: the auditor passed `--title $'line1\nline2'` and the CLI
// happily forwarded it to the server; the server stored it verbatim
// and the table renderer showed a two-line title. Newlines, CRs, and
// null bytes are all rejected here so the request never leaves the
// client. The paired server change (`internal/issues/create.go` +
// `internal/pulls/create.go`) mirrors this check for defense in depth
// and for non-CLI clients.
//
// The first return value is the cleaned title (trimmed of surrounding
// whitespace); the second is a non-nil error if the title is unusable.
func ValidateTitle(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("title is required")
	}
	for i, r := range trimmed {
		switch r {
		case '\n', '\r':
			return "", fmt.Errorf("title must be a single line (newline at offset %d)", i)
		case 0x00:
			return "", fmt.Errorf("title contains a null byte (offset %d)", i)
		}
	}
	if utf8.RuneCountInString(trimmed) > MaxTitleLen {
		return "", fmt.Errorf("title is %d characters; limit is %d", utf8.RuneCountInString(trimmed), MaxTitleLen)
	}
	return trimmed, nil
}
