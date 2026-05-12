// SPDX-License-Identifier: AGPL-3.0-or-later

package view

import (
	"io"
	"os/exec"
	"runtime"

	"github.com/cli/safeexec"
)

// osOpenArgs returns the (binary, args) tuple to open a URL on the current
// platform. Empty binary means "unsupported"; the caller surfaces a
// user-facing error in that case.
func osOpenArgs(url string) (string, []string) {
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

// runCommand exists so view_test.go can swap it out for a fake invocation
// without monkey-patching exec.Command directly.
var runCommand = func(bin string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.Command(bin, args...) //nolint:gosec // bin via safeexec; URL from validated config
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
