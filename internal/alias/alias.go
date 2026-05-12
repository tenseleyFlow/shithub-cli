// SPDX-License-Identifier: AGPL-3.0-or-later

// Package alias owns the expansion engine that turns `shithub <alias-name>`
// into `shithub <expansion> <args...>` before cobra dispatches. The
// engine is intentionally tiny: two prefix rules ("!"-prefixed = shell,
// otherwise = direct), positional-argument substitution for shell
// aliases, and a hard reject on names that collide with built-in
// commands. No alias-of-alias chaining — the simplicity prevents
// surprise loops and keeps mental model load low.
package alias

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Result captures what the dispatcher should do with the expansion.
//
//   - Shell == false: rewrite os.Args with Argv and re-enter cobra.
//   - Shell == true: spawn `sh -c <command> -- args...` (on Windows,
//     `cmd /c <command> args...`); the dispatcher handles the platform
//     branch.
type Result struct {
	Shell   bool
	Argv    []string // direct: the rewritten arg vector
	Command string   // shell: the body to run under sh -c
	Args    []string // shell: positional args that survived substitution
}

// ShellPrefix is the literal character that marks an expansion as a
// shell command. Match gh's convention so muscle memory carries over.
const ShellPrefix = "!"

// reservedNames is the set of built-in top-level commands an alias may
// not shadow. Built-ins are checked dynamically by callers (they pass
// the current cobra command tree via extraReserved), but this static
// list is the safety net used by internal/alias's own tests and by any
// future caller that forgets to pass extraReserved. Keep in lockstep
// with the verbs registered on cmd/shithub/root.go.
var reservedNames = map[string]struct{}{
	"help":       {},
	"version":    {},
	"completion": {},
	"alias":      {},
	"auth":       {},
	"api":        {},
	"config":     {},
	"repo":       {},
	"issue":      {},
	"pr":         {},
	"label":      {},
	"browse":     {},
	"search":     {},
	"status":     {},
	"org":        {},
}

// Validate returns nil iff name is a legal alias name and not reserved.
// `extraReserved` lets callers extend the deny-list with their current
// cobra command surface (e.g. "repo", "pr") so the lint stays accurate
// as the CLI grows.
func Validate(name string, extraReserved []string) error {
	if name == "" {
		return errors.New("alias: name is empty")
	}
	if strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("alias: name %q contains whitespace", name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("alias: name %q must not start with '-'", name)
	}
	if _, bad := reservedNames[name]; bad {
		return fmt.Errorf("alias: %q shadows a built-in command", name)
	}
	for _, e := range extraReserved {
		if e == name {
			return fmt.Errorf("alias: %q shadows a built-in command", name)
		}
	}
	return nil
}

// IsShell reports whether `expansion` is a shell alias.
func IsShell(expansion string) bool {
	return strings.HasPrefix(expansion, ShellPrefix)
}

// Expand applies the alias expansion rules to (expansion, args). Returns
// a Result the dispatcher can act on without further parsing.
//
// Rules:
//
//   - Direct alias: split expansion on whitespace; append `args`.
//     Example: alias `co` = `pr checkout`, invocation `shithub co 42`
//     → Argv `["pr","checkout","42"]`.
//
//   - Shell alias: strip the leading "!", substitute $1..$9 and $@ with
//     positional args (single-quote-safe; we never interpolate into the
//     command string — args are passed as separate sh arguments and the
//     command references them as $1, $@). Args not consumed via $N stay
//     available via $@.
func Expand(expansion string, args []string) (Result, error) {
	expansion = strings.TrimSpace(expansion)
	if expansion == "" {
		return Result{}, errors.New("alias: empty expansion")
	}
	if IsShell(expansion) {
		body := strings.TrimPrefix(expansion, ShellPrefix)
		return Result{Shell: true, Command: body, Args: append([]string{}, args...)}, nil
	}
	parts := strings.Fields(expansion)
	if len(parts) == 0 {
		return Result{}, errors.New("alias: expansion has no tokens")
	}
	return Result{
		Shell: false,
		Argv:  append(parts, args...),
	}, nil
}

// HasPositionalRefs reports whether a shell-alias body references any
// positional args ($1..$9 or $@). Useful for the dispatcher to decide
// whether unused args should be forwarded or warned about.
func HasPositionalRefs(body string) bool {
	if strings.Contains(body, "$@") {
		return true
	}
	for i := 1; i <= 9; i++ {
		if strings.Contains(body, "$"+strconv.Itoa(i)) {
			return true
		}
	}
	return false
}
