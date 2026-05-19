// SPDX-License-Identifier: AGPL-3.0-or-later

package upgrade

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/extension"
)

// stubGitRunner satisfies git.Runner with a no-op pull. The F40 tests
// never reach `git pull` (they short-circuit on Stat); we still need a
// non-nil Runner to pass the precondition guard inside Run.
type stubGitRunner struct{}

func (stubGitRunner) Run(_ string, _ []string, _, _ io.Writer) error { return nil }
func (stubGitRunner) Output(_ string, _ ...string) ([]byte, error)   { return nil, nil }

// TestUpgradeNonexistentExtensionErrors pins F40: `extension upgrade
// nosuch` must exit non-zero with "extension 'nosuch' is not installed",
// not the previous "skipped: not a git checkout" + exit 0 — which made
// typos look like successes.
func TestUpgradeNonexistentExtensionErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	tmp := t.TempDir()

	opts := &options{
		IO:        tf.IOStreams,
		GitRunner: stubGitRunner{},
		Name:      "nosuch",
		Dir:       tmp,
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for missing extension")
	}
	if !strings.Contains(err.Error(), "is not installed") {
		t.Errorf("error should say not installed: %v", err)
	}
}

// TestUpgradeInstalledButNotGitSkipsWithoutError pins the F40 boundary:
// an extension installed via copy-in (non-git checkout) still emits the
// friendly "skipped" notice and returns nil — the user owns the manual
// vendor, we don't fail their command.
func TestUpgradeInstalledButNotGitSkipsWithoutError(t *testing.T) {
	tf := cmdutiltest.New(t)
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, extension.Prefix+"vendored"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}

	opts := &options{
		IO:        tf.IOStreams,
		GitRunner: stubGitRunner{},
		Name:      "vendored",
		Dir:       tmp,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "not a git checkout") {
		t.Errorf("expected skip notice in stderr: %q", tf.ErrOut.String())
	}
}
