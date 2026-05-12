// SPDX-License-Identifier: AGPL-3.0-or-later

package browse

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestBrowseNoBrowserPrintsURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "",
		Repo:        "o/r",
		NoBrowser:   true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "/o/r") {
		t.Errorf("stdout: %q", tf.Out.String())
	}
}

func TestBrowseNumberToPullURL(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "42",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/pull/42") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseIssueHint(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "issue/9",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/issues/9") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseFileWithBranch(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Arg:         "README.md",
		Repo:        "o/r",
		Branch:      "dev",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/blob/dev/README.md") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseCommitFlag(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Commit:      "abc1234",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/commit/abc1234") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseCommitMutexWithPositional(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Arg:         "README.md",
		Repo:        "o/r",
		Commit:      "abc1234",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected --commit + positional mutex error")
	}
}

func TestBrowseTabsMutuallyExclusive(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return nil },
		Repo:        "o/r",
		Settings:    true,
		Wiki:        true,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected tab mutex error")
	}
}

func TestBrowseSettingsTab(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Settings:    true,
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/settings") {
		t.Errorf("URL: %q", opened)
	}
}

func TestBrowseFallsBackOnOpenerFailure(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(string) error { return errors.New("sandbox: no opener") },
		Arg:         "42",
		Repo:        "o/r",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(tf.Out.String(), "/o/r/pull/42") {
		t.Errorf("fallback URL should be on stdout: %q", tf.Out.String())
	}
	if !strings.Contains(tf.ErrOut.String(), "opener failed") {
		t.Errorf("warning missing: %q", tf.ErrOut.String())
	}
}

func TestBrowseTreeWithBranchAlone(t *testing.T) {
	tf := cmdutiltest.New(t)
	var opened string
	opts := &options{
		IO:          tf.IOStreams,
		DefaultHost: tf.Factory.DefaultHost,
		Opener:      func(s string) error { opened = s; return nil },
		Repo:        "o/r",
		Branch:      "feature/x",
	}
	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.HasSuffix(opened, "/o/r/tree/feature/x") {
		t.Errorf("URL: %q", opened)
	}
}
