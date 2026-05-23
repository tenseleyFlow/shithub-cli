// SPDX-License-Identifier: AGPL-3.0-or-later

package build

import (
	"runtime/debug"
	"testing"
)

// TestResolvedFallsBackToBuildInfo confirms I1's behavior: when
// ldflags didn't set Version/Commit/Date (the `go install ...@vX.Y.Z`
// path), Resolved() recovers what it can from debug.ReadBuildInfo().
//
// We can't easily mock runtime/debug from a test, but `go test` itself
// builds with VCS metadata embedded — so a vanilla call will exercise
// the fallback path and return non-default values for at least commit
// + date when the working tree has VCS info.
func TestResolvedFallsBackToBuildInfo(t *testing.T) {
	t.Parallel()

	version, commit, date := Resolved()

	// In a test build with no ldflags, Version starts at "dev". The
	// fallback only upgrades it when debug.BuildInfo.Main.Version is
	// non-empty and not "(devel)" — which under `go test` is usually
	// "(devel)", so Version stays "dev". This isn't a failure mode;
	// just don't assert on it.
	_ = version

	info, ok := debug.ReadBuildInfo()
	if !ok {
		t.Skip("debug.ReadBuildInfo() unavailable; cannot exercise fallback")
	}

	var wantCommit, wantDate string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			wantCommit = s.Value
		case "vcs.time":
			wantDate = s.Value
		}
	}

	if wantCommit != "" {
		if commit == "unknown" {
			t.Errorf("Commit: still 'unknown' despite vcs.revision=%q", wantCommit)
		}
		// We truncate to 12 chars; verify the prefix matches.
		if len(wantCommit) >= 12 && commit != wantCommit[:12] {
			t.Errorf("Commit: want first 12 chars of %q, got %q", wantCommit, commit)
		}
	}

	if wantDate != "" {
		if date == "unknown" {
			t.Errorf("Date: still 'unknown' despite vcs.time=%q", wantDate)
		}
	}
}

// TestResolvedPrefersLdflagsValues confirms that when ldflags HAVE
// been applied (the Make/goreleaser path), Resolved() returns those
// values verbatim — the fallback only fills in defaults, never
// overrides explicit build-time stamps.
func TestResolvedPrefersLdflagsValues(t *testing.T) {
	// Can't easily exercise this without rebuilding the test binary with
	// ldflags — document the invariant in a comment and leave the
	// behavioral guarantee to the integration test the release process
	// runs (post-goreleaser, `bin/shithub --version` must show the tag).
	t.Skip("ldflags path is covered by goreleaser CI; see I1-distribution.md test plan")
}
