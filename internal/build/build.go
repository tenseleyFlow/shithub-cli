// SPDX-License-Identifier: AGPL-3.0-or-later

// Package build exposes build-stamped identifiers for the shithub binary.
//
// The exported vars are set via -ldflags "-X ..." by the release toolchain
// (see .goreleaser.yml and the Makefile's `build` target). When the binary
// is built without ldflags (e.g., `go run`), the values fall back to
// development sentinels so we can still distinguish a dev build from a
// release build in logs and bug reports.
package build

// Version is the semantic version of this binary (e.g., "1.0.0").
// "dev" indicates an unstamped build (running from source).
var Version = "dev"

// Commit is the short git SHA the binary was built from.
// "unknown" indicates an unstamped build.
var Commit = "unknown"

// Date is the ISO-8601 timestamp at which the binary was built.
// "unknown" indicates an unstamped build.
var Date = "unknown"
