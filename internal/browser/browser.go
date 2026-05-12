// SPDX-License-Identifier: AGPL-3.0-or-later

// Package browser opens URLs in the user's default web browser. Resolves
// the platform-appropriate command via safeexec so the binary path is
// trusted; tests can swap the implementation via Override.
package browser

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/cli/safeexec"
)

// Open invokes the OS's default URL handler for `url`. Returns an error
// when no handler is available on the current platform / PATH.
func Open(url string) error {
	if opener != nil {
		return opener(url)
	}
	bin, args := platformArgs(url)
	if bin == "" {
		return fmt.Errorf("browser: no opener available for this platform")
	}
	c := exec.Command(bin, args...) //nolint:gosec // bin via safeexec; URL is a resolved string
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c.Run()
}

// Override replaces the default opener for the duration of a test.
// Returns a restore func; tests should `defer restore()` to undo.
func Override(fn func(url string) error) (restore func()) {
	prev := opener
	opener = fn
	return func() { opener = prev }
}

// opener is the injection seam used by Override. Production leaves it
// nil so Open falls through to the platform shell-out.
var opener func(url string) error

func platformArgs(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		if bin, err := safeexec.LookPath("open"); err == nil {
			return bin, []string{url}
		}
	case "windows":
		if bin, err := safeexec.LookPath("rundll32"); err == nil {
			return bin, []string{"url.dll,FileProtocolHandler", url}
		}
	default:
		if bin, err := safeexec.LookPath("xdg-open"); err == nil {
			return bin, []string{url}
		}
	}
	return "", nil
}
