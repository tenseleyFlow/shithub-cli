// SPDX-License-Identifier: AGPL-3.0-or-later

package create

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestCreateScaffolds(t *testing.T) {
	tf := cmdutiltest.New(t)
	root := t.TempDir()
	opts := &options{IO: tf.IOStreams, Name: "myext", Root: root}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "shithub-myext", "shithub-myext")); err != nil {
		t.Errorf("script missing: %v", err)
	}
}

func TestCreateRejectsPrecompiledForNow(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{IO: tf.IOStreams, Name: "myext", Precompiled: "go", Root: t.TempDir()}
	if err := Run(context.Background(), opts); err == nil {
		t.Error("expected --precompiled to be deferred-errored")
	}
}
