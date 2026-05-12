// SPDX-License-Identifier: AGPL-3.0-or-later

package markdown

import (
	"os"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// fakeEditor captures the buffer the user would have seen and replaces
// it with a scripted body. Returns a restore func that undoes the swap.
func fakeEditor(t *testing.T, body string) (capturedSeed *string, restore func()) {
	t.Helper()
	seed := ""
	prev := runEditorFn
	runEditorFn = func(_, path string) error {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		seed = string(raw)
		return os.WriteFile(path, []byte(body), 0o600)
	}
	return &seed, func() { runEditorFn = prev }
}

func TestComposeRoundTrip(t *testing.T) {
	t.Setenv("SHITHUB_EDITOR", "fake-editor")
	seed, restore := fakeEditor(t, "Hello\n\n# this is a comment\nWorld\n")
	defer restore()

	out, err := Compose(nil, ComposeOptions{
		Initial: "Initial body",
		Hints:   []string{"Lines starting with # are stripped."},
	})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !strings.HasPrefix(*seed, "Initial body") {
		t.Errorf("seed missing initial: %q", *seed)
	}
	if !strings.Contains(*seed, "# Lines starting with # are stripped.") {
		t.Errorf("seed missing hint: %q", *seed)
	}
	if out != "Hello\n\nWorld" {
		t.Errorf("output not stripped: %q", out)
	}
}

func TestComposeRespectsTemplate(t *testing.T) {
	t.Setenv("SHITHUB_EDITOR", "fake")
	seed, restore := fakeEditor(t, "result")
	defer restore()

	if _, err := Compose(nil, ComposeOptions{Template: "# heading\nbody"}); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if !strings.Contains(*seed, "# heading") {
		t.Errorf("template not seeded: %q", *seed)
	}
}

func TestComposeMissingEditorErrors(t *testing.T) {
	t.Setenv("SHITHUB_EDITOR", "")
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	t.Setenv("PATH", "/no-such-dir")
	if _, err := Compose(nil, ComposeOptions{}); err == nil {
		t.Fatal("expected error when no editor resolvable")
	}
}

func TestResolveEditorPrecedence(t *testing.T) {
	t.Setenv("SHITHUB_EDITOR", "from-shithub-env")
	t.Setenv("EDITOR", "from-editor")
	t.Setenv("VISUAL", "from-visual")
	cfgFn := func() (*config.Config, error) { return &config.Config{Editor: "from-config"}, nil }

	if got := ResolveEditor(cfgFn); got != "from-shithub-env" {
		t.Errorf("SHITHUB_EDITOR should win: %q", got)
	}
	t.Setenv("SHITHUB_EDITOR", "")
	if got := ResolveEditor(cfgFn); got != "from-config" {
		t.Errorf("config should win after env unset: %q", got)
	}
	if got := ResolveEditor(nil); got != "from-editor" {
		t.Errorf("EDITOR should win without config: %q", got)
	}
	t.Setenv("EDITOR", "")
	if got := ResolveEditor(nil); got != "from-visual" {
		t.Errorf("VISUAL fallback: %q", got)
	}
}

func TestStripCommentLines(t *testing.T) {
	in := "alpha\n# comment\n  # indented\nbeta\n"
	got := strip(in)
	if got != "alpha\nbeta" {
		t.Errorf("strip: %q", got)
	}
}
