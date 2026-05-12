// SPDX-License-Identifier: AGPL-3.0-or-later

package alias

import (
	"context"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestSetPersistsAlias(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &setOptions{
		IO:        tf.IOStreams,
		Config:    tf.Factory.Config,
		Name:      "co",
		Expansion: "pr checkout",
	}
	if err := setRun(context.Background(), opts); err != nil {
		t.Fatalf("setRun: %v", err)
	}
	cfg, _ := tf.Factory.Config()
	if cfg.Aliases["co"] != "pr checkout" {
		t.Errorf("alias not persisted; got %v", cfg.Aliases)
	}
}

func TestSetShellFlagPrependsBang(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &setOptions{
		IO:        tf.IOStreams,
		Config:    tf.Factory.Config,
		Name:      "mine",
		Expansion: "shithub pr list --author @me",
		Shell:     true,
	}
	if err := setRun(context.Background(), opts); err != nil {
		t.Fatalf("setRun: %v", err)
	}
	cfg, _ := tf.Factory.Config()
	if !strings.HasPrefix(cfg.Aliases["mine"], "!") {
		t.Errorf("--shell should prepend '!', got %q", cfg.Aliases["mine"])
	}
}

func TestSetRejectsReservedName(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &setOptions{
		IO:        tf.IOStreams,
		Config:    tf.Factory.Config,
		Name:      "auth",
		Expansion: "pr list",
	}
	if err := setRun(context.Background(), opts); err == nil {
		t.Fatal("expected error: 'auth' is a built-in name")
	}
}

func TestSetRejectsExtraReserved(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &setOptions{
		IO:           tf.IOStreams,
		Config:       tf.Factory.Config,
		Name:         "repo",
		Expansion:    "pr list",
		BuiltinNames: []string{"repo", "pr"},
	}
	if err := setRun(context.Background(), opts); err == nil {
		t.Fatal("expected error: 'repo' is among extra reserved")
	}
}

func TestListEmitsRegisteredAliases(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Aliases = map[string]string{"co": "pr checkout", "mine": "!shithub pr list"}
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	opts := &listOptions{IO: tf.IOStreams, Config: tf.Factory.Config}
	if err := listRun(context.Background(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "co:\tpr checkout") {
		t.Errorf("missing co alias in output: %q", out)
	}
	if !strings.Contains(out, "mine:\t!shithub pr list") {
		t.Errorf("missing mine alias in output: %q", out)
	}
}

func TestListEmptyAliases(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &listOptions{IO: tf.IOStreams, Config: tf.Factory.Config}
	if err := listRun(context.Background(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(tf.ErrOut.String(), "no aliases configured") {
		t.Errorf("expected empty-state message, got %q", tf.ErrOut.String())
	}
}

func TestDeleteRemovesEntry(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Aliases = map[string]string{"co": "pr checkout"}
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	opts := &deleteOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Name: "co"}
	if err := deleteRun(context.Background(), opts); err != nil {
		t.Fatalf("deleteRun: %v", err)
	}
	cfg, _ := tf.Factory.Config()
	if _, ok := cfg.Aliases["co"]; ok {
		t.Error("alias should be deleted")
	}
}

func TestDeleteAll(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Aliases = map[string]string{"co": "pr checkout", "mine": "pr list"}
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	opts := &deleteOptions{IO: tf.IOStreams, Config: tf.Factory.Config, All: true}
	if err := deleteRun(context.Background(), opts); err != nil {
		t.Fatalf("deleteRun --all: %v", err)
	}
	cfg, _ := tf.Factory.Config()
	if len(cfg.Aliases) != 0 {
		t.Errorf("expected empty aliases, got %v", cfg.Aliases)
	}
}

func TestDeleteRequiresNameOrAll(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &deleteOptions{IO: tf.IOStreams, Config: tf.Factory.Config}
	if err := deleteRun(context.Background(), opts); err == nil {
		t.Fatal("expected error when neither name nor --all")
	}
}

func TestDeleteMutuallyExclusive(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Aliases = map[string]string{"co": "pr checkout"}
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	opts := &deleteOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Name: "co", All: true}
	if err := deleteRun(context.Background(), opts); err == nil {
		t.Fatal("expected mutex error")
	}
}

func TestDeleteUnknown(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &deleteOptions{IO: tf.IOStreams, Config: tf.Factory.Config, Name: "missing"}
	if err := deleteRun(context.Background(), opts); err == nil {
		t.Fatal("expected error for missing alias")
	}
}

func TestImportFromStdin(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.In.WriteString("co: pr checkout\nmine: pr list\n")

	opts := &importOptions{
		IO: tf.IOStreams, Config: tf.Factory.Config,
		Source: "-",
	}
	if err := importRun(context.Background(), opts); err != nil {
		t.Fatalf("importRun: %v", err)
	}
	cfg, _ := tf.Factory.Config()
	if cfg.Aliases["co"] != "pr checkout" || cfg.Aliases["mine"] != "pr list" {
		t.Errorf("aliases not imported: %v", cfg.Aliases)
	}
}

func TestImportRejectsConflictWithoutClobber(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Aliases = map[string]string{"co": "existing"}
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	tf.In.WriteString("co: new value\n")

	opts := &importOptions{
		IO: tf.IOStreams, Config: tf.Factory.Config,
		Source: "-",
	}
	if err := importRun(context.Background(), opts); err == nil {
		t.Fatal("expected error on conflict without --clobber")
	}
	cfg, _ := tf.Factory.Config()
	if cfg.Aliases["co"] != "existing" {
		t.Errorf("original should be unchanged, got %q", cfg.Aliases["co"])
	}
}

func TestImportClobberOverwrites(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Config.Aliases = map[string]string{"co": "existing"}
	if err := tf.Config.Config.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	tf.In.WriteString("co: new value\n")

	opts := &importOptions{
		IO: tf.IOStreams, Config: tf.Factory.Config,
		Source: "-", Clobber: true,
	}
	if err := importRun(context.Background(), opts); err != nil {
		t.Fatalf("importRun: %v", err)
	}
	cfg, _ := tf.Factory.Config()
	if cfg.Aliases["co"] != "new value" {
		t.Errorf("clobber should overwrite, got %q", cfg.Aliases["co"])
	}
}

func TestImportRejectsReservedName(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.In.WriteString("auth: pr list\n")
	opts := &importOptions{
		IO: tf.IOStreams, Config: tf.Factory.Config,
		Source: "-",
	}
	if err := importRun(context.Background(), opts); err == nil {
		t.Fatal("expected error: alias name shadows built-in")
	}
}

func TestImportEmptyInputErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &importOptions{
		IO: tf.IOStreams, Config: tf.Factory.Config,
		Source: "-",
	}
	if err := importRun(context.Background(), opts); err == nil {
		t.Fatal("expected error on empty input")
	}
}
