// SPDX-License-Identifier: AGPL-3.0-or-later

package create

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/cli/safeexec"
)

// osOpenURL is the platform shim for `--web`. Mirrors the implementation
// in pkg/cmd/repo/view/browser.go — kept duplicated rather than promoted
// to a shared package until a third caller appears.
func osOpenURL(url string) error {
	bin, args := openerArgs(url)
	if bin == "" {
		return fmt.Errorf("issue create: no browser opener available for this platform")
	}
	c := exec.Command(bin, args...) //nolint:gosec // bin from safeexec, URL is a resolved string
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c.Run()
}

func openerArgs(url string) (string, []string) {
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
