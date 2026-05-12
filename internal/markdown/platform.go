// SPDX-License-Identifier: AGPL-3.0-or-later

package markdown

import "runtime"

// isWindows is a tiny shim so the Windows / POSIX branching in compose.go
// stays test-overridable. Hot-path is zero overhead because the runtime
// const inlines.
func isWindows() bool { return runtime.GOOS == "windows" }
