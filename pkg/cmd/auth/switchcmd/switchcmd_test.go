// SPDX-License-Identifier: AGPL-3.0-or-later

package switchcmd

import (
	"context"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
)

func TestSwitchByHostnameFlipsDefault(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("a.example").User = "u"
	tf.Config.Hosts.Get("a.example").Default = true
	tf.Config.Hosts.Get("b.example").User = "u"
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Hostname: "b.example",
	}

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	hosts, _ := tf.Factory.Hosts()
	if hosts["b.example"].Default != true {
		t.Error("b.example should now be default")
	}
	if hosts["a.example"].Default != false {
		t.Error("a.example should lose default")
	}
}

func TestSwitchSingleHostErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("only.example").User = "u"
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for single host")
	}
	if !strings.Contains(err.Error(), "nothing to switch") {
		t.Errorf("error wording: %v", err)
	}
}

func TestSwitchNoHostsErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error when no hosts")
	}
}

func TestSwitchUnknownHostErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("a.example").User = "u"
	tf.Config.Hosts.Get("b.example").User = "u"
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
		Hostname: "ghost.example",
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error for unknown host")
	}
}

func TestSwitchInteractivePromptInNonInteractiveErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("a.example").User = "u"
	tf.Config.Hosts.Get("b.example").User = "u"
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Prompter: tf.Prompt,
		Hosts:    tf.Factory.Hosts,
	}
	if err := Run(context.Background(), opts); err == nil {
		t.Fatal("expected error in non-interactive context")
	}
}
