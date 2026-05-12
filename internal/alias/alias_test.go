// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"strings"
	"testing"
)

func TestValidateAcceptsNonReserved(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"co", "bug", "mine", "pr-mine", "rebase42"} {
		if err := Validate(name, nil); err != nil {
			t.Errorf("Validate(%q): %v", name, err)
		}
	}
}

func TestValidateRejectsReserved(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"help", "auth", "api", "config", "alias", "completion", "version"} {
		if err := Validate(name, nil); err == nil {
			t.Errorf("Validate(%q) should reject built-in", name)
		}
	}
}

func TestValidateRejectsExtraReserved(t *testing.T) {
	t.Parallel()
	err := Validate("repo", []string{"repo", "pr", "issue"})
	if err == nil {
		t.Fatal("expected rejection for extra-reserved name")
	}
	if !strings.Contains(err.Error(), "built-in") {
		t.Errorf("error wording: %v", err)
	}
}

func TestValidateRejectsMalformedNames(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":        "empty",
		"co bug":  "whitespace",
		"co\tbug": "whitespace",
		"-prefix": "dash prefix",
	}
	for name, why := range cases {
		t.Run(why, func(t *testing.T) {
			if err := Validate(name, nil); err == nil {
				t.Errorf("Validate(%q) should reject (%s)", name, why)
			}
		})
	}
}

func TestIsShell(t *testing.T) {
	t.Parallel()
	if !IsShell("!echo hi") {
		t.Error("'!' prefix should mark shell alias")
	}
	if IsShell("pr checkout") {
		t.Error("plain expansion should not be shell")
	}
}

func TestExpandDirect(t *testing.T) {
	t.Parallel()
	got, err := Expand("pr checkout", []string{"42", "--force"})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if got.Shell {
		t.Error("direct expansion should not be Shell")
	}
	want := []string{"pr", "checkout", "42", "--force"}
	if len(got.Argv) != len(want) {
		t.Fatalf("Argv: want %v got %v", want, got.Argv)
	}
	for i := range want {
		if got.Argv[i] != want[i] {
			t.Errorf("Argv[%d]: want %q got %q", i, want[i], got.Argv[i])
		}
	}
}

func TestExpandShell(t *testing.T) {
	t.Parallel()
	got, err := Expand("!shithub pr list --author @me", []string{"--limit", "5"})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if !got.Shell {
		t.Fatal("expansion with '!' should be Shell")
	}
	if got.Command != "shithub pr list --author @me" {
		t.Errorf("Command: got %q", got.Command)
	}
	if len(got.Args) != 2 {
		t.Errorf("Args length: want 2 got %d (%v)", len(got.Args), got.Args)
	}
}

func TestExpandEmpty(t *testing.T) {
	t.Parallel()
	for _, exp := range []string{"", "   "} {
		if _, err := Expand(exp, nil); err == nil {
			t.Errorf("Expand(%q) should reject empty expansion", exp)
		}
	}
}

func TestExpandWhitespaceFolding(t *testing.T) {
	t.Parallel()
	got, err := Expand("pr   checkout", nil)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(got.Argv) != 2 || got.Argv[0] != "pr" || got.Argv[1] != "checkout" {
		t.Errorf("multiple spaces should collapse, got %v", got.Argv)
	}
}

func TestHasPositionalRefs(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"echo hi":         false,
		"echo $1":         true,
		"echo $@":         true,
		"echo $9 then $1": true,
		"echo no money":   false,
		"echo $0":         false, // $0 is the script name, not a positional
	}
	for body, want := range cases {
		if got := HasPositionalRefs(body); got != want {
			t.Errorf("HasPositionalRefs(%q): want %v got %v", body, want, got)
		}
	}
}
