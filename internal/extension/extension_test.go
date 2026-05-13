// SPDX-License-Identifier: AGPL-3.0-or-later

package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestListEmptyMissingDir(t *testing.T) {
	out, err := List(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestListSkipsNonPrefixedDirs(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root, "shithub-foo")
	mkdir(t, root, "gh-bar")
	mkdir(t, root, "shithub-baz")
	mkdir(t, root, "stray-file")

	got, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %d (%v)", len(got), got)
	}
	want := map[string]bool{"foo": true, "baz": true}
	for _, x := range got {
		if !want[x.Name] {
			t.Errorf("unexpected entry: %s", x.Name)
		}
	}
}

func TestFindResolvesExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit semantics differ on Windows; covered separately")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "shithub-foo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "shithub-foo")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := Find(root, "foo")
	if !ok {
		t.Fatal("Find: not found")
	}
	if got != exe {
		t.Errorf("Find: got %q want %q", got, exe)
	}
}

func TestFindRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, bad := range []string{"", "../etc", "foo/bar", "foo\\bar"} {
		if _, ok := Find(root, bad); ok {
			t.Errorf("Find(%q): should reject", bad)
		}
	}
}

func TestExecPassesArgsAndExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script exec on Windows requires a different harness")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "shithub-foo")
	_ = os.MkdirAll(dir, 0o755)
	exe := filepath.Join(dir, "shithub-foo")
	// Exit with arg count so we can check argv passthrough without
	// reading subprocess stdout.
	script := "#!/bin/sh\nexit $#\n"
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Exec(exe, []string{"a", "b", "c"}); code != 3 {
		t.Errorf("Exec: want exit 3 (argc), got %d", code)
	}
}

func TestScaffoldCreatesScript(t *testing.T) {
	root := t.TempDir()
	dir, err := Scaffold(root, "myext", false)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "shithub-myext")); err != nil {
		t.Errorf("script missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err != nil {
		t.Errorf("README missing: %v", err)
	}
}

func TestScaffoldRejectsExistingWithoutForce(t *testing.T) {
	root := t.TempDir()
	if _, err := Scaffold(root, "myext", false); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, "myext", false); err == nil {
		t.Error("Scaffold: should reject existing without --force")
	}
	if _, err := Scaffold(root, "myext", true); err != nil {
		t.Errorf("Scaffold --force: %v", err)
	}
}

func TestValidateName(t *testing.T) {
	good := []string{"foo", "my-ext", "FooBar", "x"}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q): %v", n, err)
		}
	}
	bad := []string{"", "1foo", "foo/bar", "foo bar", "../etc", "foo.bar"}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q): should reject", n)
		}
	}
}

func TestVerbFromRepo(t *testing.T) {
	cases := map[string]struct {
		verb string
		ok   bool
	}{
		"owner/shithub-foo":    {"foo", true},
		"owner/shithub-my-ext": {"my-ext", true},
		"owner/gh-foo":         {"", false},
		"shithub-foo":          {"", false}, // no owner slash
		"owner/shithub-":       {"", false}, // empty verb
		"":                     {"", false},
	}
	for in, want := range cases {
		got, ok := VerbFromRepo(in)
		if got != want.verb || ok != want.ok {
			t.Errorf("VerbFromRepo(%q): got (%q,%v) want (%q,%v)", in, got, ok, want.verb, want.ok)
		}
	}
}

func mkdir(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
}
