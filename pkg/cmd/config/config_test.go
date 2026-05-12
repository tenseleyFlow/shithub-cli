// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

func TestSetGetRoundTrip(t *testing.T) {
	tf := cmdutiltest.New(t)

	setOpts := &setOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts, Key: "editor", Value: "code -w"}
	if err := setRun(context.Background(), setOpts); err != nil {
		t.Fatalf("setRun: %v", err)
	}

	tf.Out.Reset()
	tf.ErrOut.Reset()
	getOpts := &getOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts, Key: "editor"}
	if err := getRun(context.Background(), getOpts); err != nil {
		t.Fatalf("getRun: %v", err)
	}
	if got := strings.TrimSpace(tf.Out.String()); got != "code -w" {
		t.Errorf("get output: got %q", got)
	}
}

func TestSetRejectsInvalidGitProtocol(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &setOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts, Key: "git_protocol", Value: "ftp"}
	if err := setRun(context.Background(), opts); err == nil {
		t.Fatal("expected error for bad git_protocol value")
	}
}

func TestSetRejectsUnknownKey(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &setOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts, Key: "bogus", Value: "value"}
	if err := setRun(context.Background(), opts); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestSetRejectsAliasNamespace(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &setOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts, Key: "aliases.co", Value: "pr checkout"}
	if err := setRun(context.Background(), opts); err == nil {
		t.Fatal("expected error for aliases.* via config set")
	}
}

func TestGetUnsetReturnsError(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &getOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts, Key: "editor"}
	if err := getRun(context.Background(), opts); err == nil {
		t.Fatal("expected error when key unset")
	}
}

func TestListEmitsKeyValueLines(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Editor = "vim"
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	opts := &listOptions{IO: tf.IOStreams, Config: tf.Factory.Config}
	if err := listRun(context.Background(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "editor=vim") {
		t.Errorf("expected 'editor=vim' in output, got %q", out)
	}
}

func TestListJSONShape(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Editor = "vim"
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	opts := &listOptions{IO: tf.IOStreams, Config: tf.Factory.Config, JSON: true}
	if err := listRun(context.Background(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(tf.Out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, tf.Out.String())
	}
	if got["editor"] != "vim" {
		t.Errorf("editor: got %q", got["editor"])
	}
}

func TestSetHostScopedGitProtocol(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	if err := tf.Config.Hosts.Save(); err != nil {
		t.Fatalf("save hosts: %v", err)
	}

	opts := &setOptions{
		IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts,
		Key: "git_protocol", Value: "ssh", Host: "shithub.sh",
	}
	if err := setRun(context.Background(), opts); err != nil {
		t.Fatalf("setRun host-scoped: %v", err)
	}
	hosts, _ := tf.Factory.Hosts()
	if hosts["shithub.sh"].GitProtocol != "ssh" {
		t.Errorf("git_protocol not persisted; got %+v", hosts["shithub.sh"])
	}
}

func TestSetHostScopedRejectsGlobalKey(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	if err := tf.Config.Hosts.Save(); err != nil {
		t.Fatalf("save hosts: %v", err)
	}

	opts := &setOptions{
		IO: tf.IOStreams, Config: tf.Factory.Config, Hosts: tf.Factory.Hosts,
		Key: "editor", Value: "code", Host: "shithub.sh",
	}
	if err := setRun(context.Background(), opts); err == nil {
		t.Fatal("expected error: editor is not host-scoped")
	}
}

func TestClearCache(t *testing.T) {
	tf := cmdutiltest.New(t)
	// Seed something into the cache dir to confirm removal.
	dir, err := config.CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if err := mkSeed(dir); err != nil {
		t.Fatalf("seed: %v", err)
	}

	opts := &clearCacheOptions{IO: tf.IOStreams}
	if err := clearCacheRun(context.Background(), opts); err != nil {
		t.Fatalf("clearCacheRun: %v", err)
	}
	// After clear-cache the dir should exist but be empty.
	entries, err := readDir(dir)
	if err != nil {
		t.Fatalf("readDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty cache dir, got %v", entries)
	}
}
