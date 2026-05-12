// SPDX-License-Identifier: AGPL-3.0-or-later

// Package markdown owns small helpers that bridge the user's preferred
// editor and the raw markdown the API expects. The exported Compose
// function spawns the configured editor with a templated buffer, strips
// comment lines on save, and returns the result.
package markdown

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cli/safeexec"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// CommentMarker is the prefix on lines stripped from the editor buffer
// before send. Mirrors `git commit`'s convention so muscle memory carries
// across tools.
const CommentMarker = "#"

// ComposeOptions configures a single Compose invocation. Fields are all
// optional except the file extension (defaults to ".md").
type ComposeOptions struct {
	// Initial body presented to the editor. Empty is fine.
	Initial string
	// Template content appended (or replacing the initial) before
	// the comment header. Used for issue-template flows.
	Template string
	// Hints rendered as `# ` comment lines at the bottom — instructions
	// the user sees but never sends. Add things like "Comments starting
	// with # are removed." here.
	Hints []string
	// Extension controls the temp filename suffix so the editor picks up
	// syntax highlighting. Defaults to ".md".
	Extension string
	// EditorOverride forces a specific editor binary. When empty, falls
	// back to ResolveEditor.
	EditorOverride string
}

// Compose spawns the configured editor with a seeded buffer, waits, and
// returns the user-saved content with comment lines removed and
// surrounding whitespace trimmed. The temp file is removed on exit.
func Compose(cfgFn func() (*config.Config, error), opts ComposeOptions) (string, error) {
	editor := opts.EditorOverride
	if editor == "" {
		editor = ResolveEditor(cfgFn)
	}
	if editor == "" {
		return "", fmt.Errorf("markdown: no editor configured; set $EDITOR or `shithub config set editor`")
	}

	ext := opts.Extension
	if ext == "" {
		ext = ".md"
	}

	dir, err := os.MkdirTemp("", "shithub-compose-")
	if err != nil {
		return "", fmt.Errorf("markdown: mkdir tmp: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "EDITMSG"+ext)
	body := composeSeed(opts)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", fmt.Errorf("markdown: write seed: %w", err)
	}

	if err := runEditor(editor, path); err != nil {
		return "", err
	}

	raw, err := os.ReadFile(path) //nolint:gosec // path is a temp file we own
	if err != nil {
		return "", fmt.Errorf("markdown: read edited: %w", err)
	}
	return strip(string(raw)), nil
}

// composeSeed concatenates Initial / Template / Hints into the buffer
// shown to the user. The hint block is always pushed to the bottom so
// the cursor lands on the writable area.
func composeSeed(opts ComposeOptions) string {
	var b strings.Builder
	if opts.Initial != "" {
		b.WriteString(opts.Initial)
		if !strings.HasSuffix(opts.Initial, "\n") {
			b.WriteByte('\n')
		}
	}
	if opts.Template != "" {
		b.WriteString(opts.Template)
		if !strings.HasSuffix(opts.Template, "\n") {
			b.WriteByte('\n')
		}
	}
	if len(opts.Hints) > 0 {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		for _, h := range opts.Hints {
			b.WriteString(CommentMarker + " ")
			b.WriteString(h)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// strip removes any line whose first non-whitespace byte is '#' and
// trims surrounding whitespace. The check is intentionally simple — the
// rule is "leading '#' marks a comment", same as git commit.
func strip(body string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimLeft(ln, " \t")
		if strings.HasPrefix(t, CommentMarker) {
			continue
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// ResolveEditor picks an editor binary using gh's precedence:
//
//  1. SHITHUB_EDITOR env var
//  2. config.editor (`shithub config set editor "<value>"`)
//  3. EDITOR env var
//  4. VISUAL env var
//  5. nano (POSIX) / notepad (Windows)
//
// Returns the command line string verbatim so the user can include flags
// (e.g., "code -w"); runEditor passes it through `sh -c` accordingly.
func ResolveEditor(cfgFn func() (*config.Config, error)) string {
	if v := os.Getenv("SHITHUB_EDITOR"); v != "" {
		return v
	}
	if cfgFn != nil {
		if cfg, err := cfgFn(); err == nil && cfg != nil && cfg.Editor != "" {
			return cfg.Editor
		}
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if _, err := safeexec.LookPath("nano"); err == nil {
		return "nano"
	}
	if _, err := safeexec.LookPath("notepad"); err == nil {
		return "notepad"
	}
	return ""
}

// runEditor spawns the resolved editor command line via `sh -c` on
// POSIX / `cmd /c` on Windows. Stdin/stdout/stderr are inherited so the
// TUI editor (vim, nano) renders directly in the parent terminal.
//
// The variable indirection through runEditorFn lets tests fake the
// editor without spawning a real subprocess.
var runEditorFn = func(editor, path string) error {
	// Quote the path so file names with spaces survive the shell hop.
	cmdline := editor + " " + shellQuote(path)
	bin, args := shellInvoke(cmdline)
	c := exec.Command(bin, args...) //nolint:gosec // bin is hardcoded sh/cmd; args come from a trusted editor config
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("markdown: editor exit: %w", err)
	}
	return nil
}

func runEditor(editor, path string) error { return runEditorFn(editor, path) }

// shellQuote performs minimal single-quote wrapping so spaces in path
// survive the sh/cmd hop. Embedded single quotes get the standard
// "'\”" escape on POSIX.
func shellQuote(s string) string {
	if !strings.ContainsAny(s, ` 	'"`) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellInvoke returns (bin, args) for running a shell command string.
// POSIX systems use sh -c; Windows uses cmd /c. The runtime check is
// pushed to a tiny helper so the editor surface remains portable.
func shellInvoke(cmdline string) (string, []string) {
	if isWindows() {
		return "cmd", []string{"/c", cmdline}
	}
	return "sh", []string{"-c", cmdline}
}
