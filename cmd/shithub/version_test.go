// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/build"
)

// TestPrintVersionHuman locks the human-readable format that release
// notes and shell scripts may parse. Format is "shithub <ver> (<sha>) built <date>".
// I1: printVersion now sources via build.Resolved() so go-install builds
// recover VCS metadata from runtime/debug.ReadBuildInfo(); assert against
// Resolved() instead of the bare ldflags vars.
func TestPrintVersionHuman(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printVersion(&buf, false); err != nil {
		t.Fatalf("printVersion: %v", err)
	}

	version, commit, date := build.Resolved()
	got := buf.String()
	want := "shithub " + version + " (" + commit + ") built " + date + "\n"
	if got != want {
		t.Fatalf("human output mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

// TestPrintVersionJSON verifies the JSON shape and field-name stability.
// Tooling depends on these keys; renaming is a breaking change.
func TestPrintVersionJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printVersion(&buf, true); err != nil {
		t.Fatalf("printVersion: %v", err)
	}

	var info versionInfo
	if err := json.Unmarshal(buf.Bytes(), &info); err != nil {
		t.Fatalf("json decode: %v\nraw: %s", err, buf.String())
	}

	wantVersion, wantCommit, wantDate := build.Resolved()
	if info.Version != wantVersion {
		t.Errorf("Version: want %q got %q", wantVersion, info.Version)
	}
	if info.Commit != wantCommit {
		t.Errorf("Commit: want %q got %q", wantCommit, info.Commit)
	}
	if info.Date != wantDate {
		t.Errorf("Date: want %q got %q", wantDate, info.Date)
	}
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion: want %q got %q", runtime.Version(), info.GoVersion)
	}
	if info.OS != runtime.GOOS {
		t.Errorf("OS: want %q got %q", runtime.GOOS, info.OS)
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("Arch: want %q got %q", runtime.GOARCH, info.Arch)
	}

	// Pretty-printed JSON must be 2-space indented.
	if !strings.Contains(buf.String(), "  \"version\":") {
		t.Errorf("expected 2-space indented JSON, got: %s", buf.String())
	}
}
