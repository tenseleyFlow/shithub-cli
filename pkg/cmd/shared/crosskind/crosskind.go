// SPDX-License-Identifier: AGPL-3.0-or-later

// Package crosskind centralizes the shared-namespace verb routing
// helper. Issues and PRs share a per-repo number namespace; when a
// user runs `shithub issue close <PR#>` or `shithub pr ready <issue#>`,
// the verb should detect the wrong-side case and surface a friendly
// redirect message, not the server's raw error.
//
// H2 (from the H-audit): pre-fix every state-mutating verb on the
// wrong side leaked either a raw 422 or "pull request not found". Only
// `view` did the polite redirect. This package extracts that pattern
// into a single helper used by every shared-namespace verb.
package crosskind

import (
	"context"
	"errors"
	"fmt"

	"github.com/tenseleyFlow/shithub-cli/internal/issues"
	"github.com/tenseleyFlow/shithub-cli/internal/pulls"
)

// ErrWrongNamespace is returned by Check when the given number resolves
// to the opposite kind from what the caller expected. Callers format
// the result via the typed redirect message — see Format.
//
// Wraps a sentinel so callers can errors.Is-check.
type ErrWrongNamespace struct {
	// Number is the number that was looked up.
	Number int
	// CmdName is the friendly command name passed to Check (e.g.
	// "issue close", "pr ready"). Surfaced in the formatted message.
	CmdName string
	// Found is the kind we actually saw — "issue" or "pr". The
	// suggestion redirects the user to the matching command tree.
	Found string
	// SuggestedVerb is the command tail to suggest (e.g. "close",
	// "ready"). Defaults to the verb portion of CmdName when empty.
	SuggestedVerb string
	// Asymmetric marks verbs that exist on only one side of the
	// namespace (e.g. `pr ready`, `pr merge`, `pr review`). When
	// true, the formatted message explains the constraint instead
	// of pointing at an `issue <verb>` that doesn't exist.
	// audit-I6: pre-fix `pr ready 1` on an issue suggested
	// `try shithub issue ready 1` — but `issue ready` doesn't
	// exist, so the user chased a dead command.
	Asymmetric bool
}

// errWrongNamespaceSentinel is the marker for errors.Is matching;
// ErrWrongNamespace's Is method compares against it.
var errWrongNamespaceSentinel = errors.New("cross-namespace verb on wrong side")

func (e *ErrWrongNamespace) Error() string {
	suggested := e.SuggestedVerb
	if suggested == "" {
		suggested = "view"
	}
	tree := "pr"
	kindNoun := "issue"
	if e.Found == "issue" {
		tree = "issue"
	} else {
		kindNoun = "pull request"
	}
	// audit-I6: asymmetric verbs (e.g. `pr ready`, `pr merge`,
	// `pr review`) don't exist on the other side of the namespace.
	// Pre-fix we still emitted "try `shithub issue ready N`" — a
	// dead command. Now we explain the asymmetry and steer the
	// user at `view` (which exists everywhere) for inspection.
	if e.Asymmetric {
		return fmt.Sprintf("%s: #%d is %s %s; `%s` only applies to %ss (run `shithub %s view %d` to inspect)",
			e.CmdName, e.Number, articleFor(kindNoun), kindNoun,
			e.CmdName, asymmetricSubjectFor(e.CmdName), tree, e.Number)
	}
	return fmt.Sprintf("%s: #%d is %s %s; try `shithub %s %s %d`",
		e.CmdName, e.Number, articleFor(kindNoun), kindNoun, tree, suggested, e.Number)
}

// asymmetricSubjectFor returns the noun-form ("pull request" or
// "issue") the asymmetric verb operates on, derived from the
// command prefix in CmdName. Used by Error() to render messages
// like "`pr ready` only applies to pull requests".
func asymmetricSubjectFor(cmdName string) string {
	switch {
	case len(cmdName) >= 3 && cmdName[:3] == "pr ":
		return "pull request"
	case len(cmdName) >= 6 && cmdName[:6] == "issue ":
		return "issue"
	}
	return "the calling kind"
}

// Is supports errors.Is matching against the sentinel — callers that
// don't need the typed fields can write `errors.Is(err, ErrWrongNamespaceSentinel)`.
func (e *ErrWrongNamespace) Is(target error) bool {
	return target == errWrongNamespaceSentinel
}

// articleFor picks "an" before vowel sounds, "a" otherwise.
func articleFor(noun string) string {
	if noun == "" {
		return "a"
	}
	switch noun[0] {
	case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
		return "an"
	}
	return "a"
}

// Check verifies that `number` in (owner/name) is on the expected side
// of the namespace. expectedKind must be "issue" or "pr".
//
// Behavior:
//   - If the number is on the expected side (or the lookup errors
//     non-NotFound), returns nil. Callers proceed with the mutation.
//   - If the number is on the OPPOSITE side, returns *ErrWrongNamespace
//     with a pre-formatted message.
//   - If the number doesn't exist at all, returns nil — the caller's
//     downstream mutation will surface the "not found".
//
// Wires through one extra API call before each state-mutating verb on
// a number argument. That's cheap (single GET to the opposite client)
// and runs only on the wrong-side path because the expected-side
// lookup is what the mutation already needs to do.
func Check(ctx context.Context, ic *issues.Client, pc *pulls.Client, owner, name string, number int, expectedKind, cmdName, suggestedVerb string) error {
	return CheckAsymmetric(ctx, ic, pc, owner, name, number, expectedKind, cmdName, suggestedVerb, false)
}

// CheckAsymmetric is Check with an explicit `asymmetric` flag for
// verbs that exist on only one side of the namespace (e.g.
// `pr ready`, `pr merge`, `pr review`, `pr checkout`, `pr diff`,
// `pr update-branch`). When asymmetric=true and the wrong-side
// case fires, the error message explains the constraint instead
// of pointing at an `issue <verb>` that doesn't exist. audit-I6.
func CheckAsymmetric(ctx context.Context, ic *issues.Client, pc *pulls.Client, owner, name string, number int, expectedKind, cmdName, suggestedVerb string, asymmetric bool) error {
	if number <= 0 {
		return nil
	}
	switch expectedKind {
	case "issue":
		// The caller will try the issue endpoint. Pre-flight: see if
		// this number is actually a PR. If yes, friendly redirect; if
		// no, return nil and let the caller's call proceed normally.
		if pc == nil {
			return nil
		}
		if _, err := pc.View(ctx, owner, name, number); err == nil {
			return &ErrWrongNamespace{
				Number: number, CmdName: cmdName,
				Found: "pr", SuggestedVerb: suggestedVerb,
				Asymmetric: asymmetric,
			}
		}
	case "pr":
		if ic == nil {
			return nil
		}
		if _, err := ic.View(ctx, owner, name, number); err == nil {
			return &ErrWrongNamespace{
				Number: number, CmdName: cmdName,
				Found: "issue", SuggestedVerb: suggestedVerb,
				Asymmetric: asymmetric,
			}
		}
	default:
		return fmt.Errorf("crosskind.Check: invalid expectedKind %q", expectedKind)
	}
	return nil
}

// IsWrongNamespace reports whether err is *ErrWrongNamespace. Convenience
// for callers that want to surface the message verbatim and exit non-zero
// without their own error-type dispatch.
func IsWrongNamespace(err error) bool {
	if err == nil {
		return false
	}
	var w *ErrWrongNamespace
	return errors.As(err, &w)
}

// Compile-time error interface assertion.
var _ error = (*ErrWrongNamespace)(nil)
