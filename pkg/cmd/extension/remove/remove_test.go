// SPDX-License-Identifier: AGPL-3.0-or-later

package remove

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestRemoveDeletesExtensionDir(t *testing.T) {
	tf := cmdutiltest.New(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "shithub-foo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	opts := &options{IO: tf.IOStreams, Name: "foo", Dir: dir}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("dir not removed: %v", err)
	}
}

func TestRemoveMissing(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{IO: tf.IOStreams, Name: "nope", Dir: t.TempDir()}
	if err := Run(context.Background(), opts); err == nil {
		t.Error("expected error for missing extension")
	}
}

func TestRemoveRejectsBadName(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{IO: tf.IOStreams, Name: "../etc", Dir: t.TempDir()}
	if err := Run(context.Background(), opts); err == nil {
		t.Error("expected ValidateName error")
	}
}
