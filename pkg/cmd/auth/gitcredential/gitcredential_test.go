// SPDX-License-Identifier: AGPL-3.0-or-later

package gitcredential

import (
	"context"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

func TestGitCredentialGetWritesProtocolResponse(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	_ = config.SetToken(tf.Config.Keyring, "shithub.sh", "mf", "shithub_pat_xxx")
	_ = tf.Config.Hosts.Save()

	tf.In.WriteString("protocol=https\nhost=shithub.sh\n\n")

	opts := &Options{
		IO:      tf.IOStreams,
		Hosts:   tf.Factory.Hosts,
		Keyring: tf.Factory.Keyring,
	}
	if err := Run(context.Background(), opts, "get"); err != nil {
		t.Fatalf("Run get: %v", err)
	}
	out := tf.Out.String()
	if !strings.Contains(out, "username=mf\n") {
		t.Errorf("missing username line: %q", out)
	}
	if !strings.Contains(out, "password=shithub_pat_xxx\n") {
		t.Errorf("missing password line: %q", out)
	}
}

func TestGitCredentialGetUnknownHostSilent(t *testing.T) {
	tf := cmdutiltest.New(t)
	tf.In.WriteString("protocol=https\nhost=ghost.example\n\n")

	opts := &Options{
		IO:      tf.IOStreams,
		Hosts:   tf.Factory.Hosts,
		Keyring: tf.Factory.Keyring,
	}
	if err := Run(context.Background(), opts, "get"); err != nil {
		t.Fatalf("Run get: %v", err)
	}
	if tf.Out.Len() != 0 {
		t.Errorf("missing host should yield silent fall-through, got %q", tf.Out.String())
	}
}

func TestGitCredentialStoreEraseNoops(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &Options{
		IO:      tf.IOStreams,
		Hosts:   tf.Factory.Hosts,
		Keyring: tf.Factory.Keyring,
	}
	for _, verb := range []string{"store", "erase"} {
		if err := Run(context.Background(), opts, verb); err != nil {
			t.Errorf("%s should be a no-op, got: %v", verb, err)
		}
	}
}

func TestGitCredentialUnknownVerbErrors(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &Options{
		IO:      tf.IOStreams,
		Hosts:   tf.Factory.Hosts,
		Keyring: tf.Factory.Keyring,
	}
	if err := Run(context.Background(), opts, "wat"); err == nil {
		t.Fatal("expected error on unknown verb")
	}
}
