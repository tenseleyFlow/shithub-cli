// SPDX-License-Identifier: AGPL-3.0-or-later

package list

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestListRendersInstalled(t *testing.T) {
	tf := cmdutiltest.New(t)
	dir := t.TempDir()
	for _, name := range []string{"shithub-foo", "shithub-bar"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	opts := &options{IO: tf.IOStreams, Dir: dir}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, want := range []string{"foo", "bar"} {
		if !strings.Contains(tf.Out.String(), want) {
			t.Errorf("table missing %q: %s", want, tf.Out.String())
		}
	}
}

func TestListEmpty(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{IO: tf.IOStreams, Dir: filepath.Join(t.TempDir(), "missing")}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "no extensions") {
		t.Errorf("empty-state message missing: %q", tf.ErrOut.String())
	}
}
