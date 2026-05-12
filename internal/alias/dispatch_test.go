// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"runtime"
	"strings"
	"testing"
)

func TestDispatchDirectExpansion(t *testing.T) {
	t.Parallel()
	res, ok, err := Dispatch(
		[]string{"co", "42"},
		map[string]string{"co": "pr checkout"},
		[]string{"auth", "api"},
	)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !ok {
		t.Fatal("expected match")
	}
	want := []string{"pr", "checkout", "42"}
	if len(res.Argv) != len(want) {
		t.Fatalf("Argv: want %v got %v", want, res.Argv)
	}
	for i := range want {
		if res.Argv[i] != want[i] {
			t.Errorf("Argv[%d]: want %q got %q", i, want[i], res.Argv[i])
		}
	}
}

func TestDispatchBuiltinWins(t *testing.T) {
	t.Parallel()
	// Even though "auth" is registered as an alias, the built-in must win.
	_, ok, err := Dispatch(
		[]string{"auth", "login"},
		map[string]string{"auth": "pr list"},
		[]string{"auth", "api"},
	)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if ok {
		t.Error("built-in should always shadow alias")
	}
}

func TestDispatchNoMatch(t *testing.T) {
	t.Parallel()
	_, ok, _ := Dispatch(
		[]string{"unknown"},
		map[string]string{"co": "pr checkout"},
		nil,
	)
	if ok {
		t.Error("no alias should match")
	}
}

func TestDispatchEmptyAliases(t *testing.T) {
	t.Parallel()
	_, ok, _ := Dispatch([]string{"co"}, nil, nil)
	if ok {
		t.Error("nil aliases shouldn't match")
	}
}

func TestDispatchShellAlias(t *testing.T) {
	t.Parallel()
	res, ok, err := Dispatch(
		[]string{"mine", "--limit", "5"},
		map[string]string{"mine": "!shithub pr list --author @me"},
		nil,
	)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !ok || !res.Shell {
		t.Fatal("expected shell-alias match")
	}
	if res.Command != "shithub pr list --author @me" {
		t.Errorf("Command: got %q", res.Command)
	}
	if len(res.Args) != 2 {
		t.Errorf("Args: got %v", res.Args)
	}
}

func TestFirstNonFlagIndex(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"co"}, 0},
		{[]string{"--help"}, -1},
		{[]string{"--", "co"}, 1},
		{[]string{"-x", "co"}, 1},
		{[]string{}, -1},
	}
	for _, tc := range cases {
		if got := firstNonFlagIndex(tc.args); got != tc.want {
			t.Errorf("firstNonFlagIndex(%v): want %d got %d", tc.args, tc.want, got)
		}
	}
}

func TestBuildShellCommandUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only branch")
	}
	se, err := BuildShellCommand("echo hi", []string{"a", "b"})
	if err != nil {
		t.Fatalf("BuildShellCommand: %v", err)
	}
	if !strings.HasSuffix(se.Binary, "/sh") && se.Binary != "/bin/sh" && !strings.Contains(se.Binary, "sh") {
		t.Errorf("Binary should be sh, got %q", se.Binary)
	}
	// Args should start with -c, body, --
	if len(se.Args) < 3 || se.Args[0] != "-c" || se.Args[1] != "echo hi" || se.Args[2] != "--" {
		t.Errorf("Args head: got %v", se.Args)
	}
}
