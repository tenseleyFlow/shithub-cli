// SPDX-License-Identifier: AGPL-3.0-or-later

package logout

import (
	"context"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

func TestLogoutSingleHostNoFlagDeletes(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	_ = config.SetToken(tf.Config.Keyring, "shithub.sh", "mf", "shithub_pat_xxx")
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
		Yes:      true,
	}

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	hosts, _ := tf.Factory.Hosts()
	if _, ok := hosts["shithub.sh"]; ok {
		t.Error("hosts entry should be gone after logout")
	}
	if tf.Config.Keyring.Has("shithub:shithub.sh", "mf") {
		t.Error("keyring entry should be gone after logout")
	}
}

func TestLogoutMultiHostRequiresFlag(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("a.example").User = "u"
	tf.Config.Hosts.Get("b.example").User = "u"
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
		Yes:      true,
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when multi-host without --hostname")
	}
	if !strings.Contains(err.Error(), "--hostname") {
		t.Errorf("error should hint at --hostname, got: %v", err)
	}
}

func TestLogoutMissingHostError(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
		Yes:      true,
		Hostname: "ghost.example",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error for unknown host")
	}
}

func TestLogoutWithoutYesInNonInteractiveErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error without --yes in non-interactive")
	}
}
