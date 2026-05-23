// SPDX-License-Identifier: AGPL-3.0-or-later

// Package build exposes build-stamped identifiers for the shithub binary.
//
// The exported vars are set via -ldflags "-X ..." by the release toolchain
// (see .goreleaser.yml and the Makefile's `build` target). When the binary
// is built without ldflags (e.g., `go run`, `go install ...@latest`), the
// values fall back to development sentinels so we can still distinguish a
// dev build from a release build in logs and bug reports.
//
// I1: when invoked through `go install github.com/.../cmd/shithub@vX.Y.Z`,
// the ldflags path doesn't fire — the module version and VCS metadata is
// instead embedded in runtime/debug.ReadBuildInfo(). Resolved() prefers
// the ldflags values when set and falls back to the build info, so users
// who got their binary via `go install` see a useful version string too
// (pre-fix every non-Make build reported "shithub dev (unknown) built
// unknown").
package build

import (
	"runtime/debug"
	"strings"
	"sync"
)

// Version is the semantic version of this binary (e.g., "1.0.0").
// "dev" indicates an unstamped build (running from source).
var Version = "dev"

// Commit is the short git SHA the binary was built from.
// "unknown" indicates an unstamped build.
var Commit = "unknown"

// Date is the ISO-8601 timestamp at which the binary was built.
// "unknown" indicates an unstamped build.
var Date = "unknown"

// Resolved returns the effective build info: the -ldflags-set values
// when present, otherwise the runtime/debug.ReadBuildInfo() fallback.
//
// The fallback covers `go install github.com/.../cmd/shithub@vX.Y.Z`:
// Go embeds the module version + VCS metadata into the binary even
// without ldflags, and we can recover it at runtime.
//
// Precedence per field:
//   - Version: ldflags value if not "dev", else debug.BuildInfo.Main.Version
//     (with the leading "v" trimmed for consistency with semver tags).
//   - Commit:  ldflags value if not "unknown", else the vcs.revision
//     setting (truncated to 12 chars for display).
//   - Date:    ldflags value if not "unknown", else the vcs.time setting.
//
// Results are cached after the first call — debug.ReadBuildInfo's output
// is invariant for the lifetime of the process.
func Resolved() (version, commit, date string) {
	resolveOnce.Do(func() {
		resolvedVersion, resolvedCommit, resolvedDate = resolveBuildInfo()
	})
	return resolvedVersion, resolvedCommit, resolvedDate
}

var (
	resolveOnce                                          sync.Once
	resolvedVersion, resolvedCommit, resolvedDate string
)

func resolveBuildInfo() (version, commit, date string) {
	version, commit, date = Version, Commit, Date

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	// Module version: when installed via `go install ...@v1.2.3`,
	// info.Main.Version is the tag with a leading "v". Bare "(devel)"
	// shows up for replace-directive builds and means "I don't know" —
	// don't downgrade the ldflags value with it.
	if version == "dev" {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = strings.TrimPrefix(v, "v")
		}
	}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "unknown" && s.Value != "" {
				commit = s.Value
				if len(commit) > 12 {
					commit = commit[:12]
				}
			}
		case "vcs.time":
			if date == "unknown" && s.Value != "" {
				date = s.Value
			}
		}
	}
	return
}
