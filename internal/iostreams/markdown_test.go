// SPDX-License-Identifier: AGPL-3.0-or-later

package iostreams

import (
	"strings"
	"testing"
)

func TestRenderMarkdownDisabledReturnsRaw(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	// Test() defaults to color-off and non-TTY; both should short-circuit.

	const src = "# Heading\n\nBody **bold**."
	got, err := s.RenderMarkdown(src)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if got != src {
		t.Errorf("disabled path should return source verbatim\nwant: %q\ngot:  %q", src, got)
	}
}

func TestRenderMarkdownEnabledTransforms(t *testing.T) {
	t.Parallel()
	s, _, _, _ := Test()
	s.SetColorEnabled(true)
	s.SetStdoutTTY(true)

	const src = "# Heading\n\nBody."
	got, err := s.RenderMarkdown(src)
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	// We don't assert on the exact bytes (theme-dependent) — only that
	// glamour ran and the original payload made it through.
	if !strings.Contains(got, "Heading") || !strings.Contains(got, "Body") {
		t.Errorf("rendered markdown should preserve content, got: %q", got)
	}
}

func TestRenderMarkdownGlamourStyleEnv(t *testing.T) {
	// We can't easily snapshot glamour's themed output, but we can verify
	// the call path doesn't error out when the env var is set.
	t.Setenv(EnvGlamourStyle, "dark")

	s, _, _, _ := Test()
	s.SetColorEnabled(true)
	s.SetStdoutTTY(true)

	if _, err := s.RenderMarkdown("# X"); err != nil {
		t.Fatalf("RenderMarkdown with GLAMOUR_STYLE=dark: %v", err)
	}
}
