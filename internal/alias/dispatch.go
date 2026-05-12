// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/cli/safeexec"
)

// Dispatch decides whether `args` should be rewritten via an alias. The
// returned Result captures the rewrite:
//
//   - Result.Shell == false, Result.Argv is the new arg slice the caller
//     should hand back to cobra. The first element is the verb.
//   - Result.Shell == true, Result.Command + Result.Args describe a shell
//     invocation the caller should spawn (use the package-level
//     RunShellCommand helper, or implement the spawn locally).
//   - matched == false: no alias matched; the caller proceeds with the
//     original args unchanged.
//
// `builtins` are the top-level cobra command names that must never be
// shadowed. If the first non-flag argument names a built-in, Dispatch
// returns matched=false even when an alias of the same name exists —
// the built-in always wins.
func Dispatch(args []string, aliases map[string]string, builtins []string) (Result, bool, error) {
	if len(aliases) == 0 {
		return Result{}, false, nil
	}
	verbIdx := firstNonFlagIndex(args)
	if verbIdx < 0 {
		return Result{}, false, nil
	}
	verb := args[verbIdx]
	if isBuiltin(verb, builtins) {
		return Result{}, false, nil
	}
	expansion, ok := aliases[verb]
	if !ok {
		return Result{}, false, nil
	}
	tail := append([]string(nil), args[:verbIdx]...)
	res, err := Expand(expansion, args[verbIdx+1:])
	if err != nil {
		return Result{}, false, err
	}
	if !res.Shell {
		// Argv is [expanded-words..., trailing args]; prepend any flags
		// that came before the verb (rare, but supported for parity).
		res.Argv = append(tail, res.Argv...)
	}
	return res, true, nil
}

// firstNonFlagIndex returns the index of the first arg that doesn't look
// like a flag, or -1 if every arg is a flag. We use a string-prefix check
// rather than full cobra parsing because alias dispatch happens BEFORE
// cobra sees the args.
func firstNonFlagIndex(args []string) int {
	for i, a := range args {
		if a == "--" {
			// After "--", every remaining arg is positional. The verb,
			// if any, would be args[i+1].
			if i+1 < len(args) {
				return i + 1
			}
			return -1
		}
		if !strings.HasPrefix(a, "-") {
			return i
		}
	}
	return -1
}

// isBuiltin reports whether name is in builtins. Linear scan is fine —
// the built-in set is tiny.
func isBuiltin(name string, builtins []string) bool {
	for _, b := range builtins {
		if b == name {
			return true
		}
	}
	return false
}

// ShellExecutor describes the platform-specific exec command for a
// shell alias body. Public so the dispatcher in cmd/shithub can spawn
// via its preferred mechanism (we don't import os/exec from main if
// avoidable, but here it's the simplest seam).
type ShellExecutor struct {
	Binary string
	Args   []string
}

// BuildShellCommand assembles the exec args for running a shell alias
// body. On Unix: sh -c <body> -- <args...>; on Windows: cmd /c <body>
// <args...>. Returns an error if the shell binary is not on PATH.
func BuildShellCommand(body string, args []string) (ShellExecutor, error) {
	if runtime.GOOS == "windows" {
		bin, err := safeexec.LookPath("cmd")
		if err != nil {
			return ShellExecutor{}, err
		}
		full := []string{"/c", body}
		full = append(full, args...)
		return ShellExecutor{Binary: bin, Args: full}, nil
	}
	bin, err := safeexec.LookPath("sh")
	if err != nil {
		return ShellExecutor{}, err
	}
	full := []string{"-c", body, "--"}
	full = append(full, args...)
	return ShellExecutor{Binary: bin, Args: full}, nil
}

// Cmd builds an *exec.Cmd ready to Run/Start. Exposed so the caller
// can wire stdio without us pulling iostreams into this package.
func (s ShellExecutor) Cmd() *exec.Cmd {
	return exec.Command(s.Binary, s.Args...) //nolint:gosec // body is user-configured alias; that's the whole point
}
