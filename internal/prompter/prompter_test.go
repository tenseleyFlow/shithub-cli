// SPDX-License-Identifier: AGPL-3.0-or-later

package prompter

import (
	"errors"
	"testing"
)

func TestAbortedError(t *testing.T) {
	t.Parallel()
	var err error = Aborted{}
	if err.Error() != "prompter: user aborted" {
		t.Errorf("Aborted message: got %q", err.Error())
	}
	// As-conversion sanity: callers will errors.As against the typed
	// abort error to translate to a clean exit.
	var ab Aborted
	if !errors.As(err, &ab) {
		t.Error("errors.As should match Aborted")
	}
}

func TestNotInteractiveError(t *testing.T) {
	t.Parallel()
	defaultMsg := NotInteractive{}.Error()
	if defaultMsg == "" {
		t.Error("default NotInteractive should have a message")
	}
	withReason := NotInteractive{Reason: "stdin is a pipe"}.Error()
	if withReason != "prompter: stdin is a pipe" {
		t.Errorf("with reason: got %q", withReason)
	}
}
