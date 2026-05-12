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
func TestPrintVersionHuman(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printVersion(&buf, false); err != nil {
		t.Fatalf("printVersion: %v", err)
	}

	got := buf.String()
	want := "shithub " + build.Version + " (" + build.Commit + ") built " + build.Date + "\n"
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

	if info.Version != build.Version {
		t.Errorf("Version: want %q got %q", build.Version, info.Version)
	}
	if info.Commit != build.Commit {
		t.Errorf("Commit: want %q got %q", build.Commit, info.Commit)
	}
	if info.Date != build.Date {
		t.Errorf("Date: want %q got %q", build.Date, info.Date)
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
