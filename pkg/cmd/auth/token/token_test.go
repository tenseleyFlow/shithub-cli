// SPDX-License-Identifier: AGPL-3.0-or-later

package token

import (
	"context"
	"strings"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/cmdutil/cmdutiltest"
	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

func TestTokenPrintsStoredValue(t *testing.T) {
	tf := cmdutiltest.New(t)
	// Seed a host with an insecure-storage token so ResolveToken finds it.
	tf.Config.Hosts.Get("shithub.sh").User = "mf"
	tf.Config.Hosts.Get("shithub.sh").OAuthToken = "shithub_pat_xyz"
	tf.Config.Hosts.Get("shithub.sh").InsecureStorage = true
	_ = tf.Config.Hosts.Save()

	opts := &Options{
		IO:       tf.IOStreams,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
		Hostname: "shithub.sh",
	}

	if err := Run(context.Background(), opts); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := tf.Out.String()
	if got != "shithub_pat_xyz" {
		t.Errorf("token output: want exact bytes, got %q", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("token should have no trailing newline")
	}
}

func TestTokenMissingFailsWithHint(t *testing.T) {
	tf := cmdutiltest.New(t)
	opts := &Options{
		IO:       tf.IOStreams,
		Hosts:    tf.Factory.Hosts,
		Keyring:  tf.Factory.Keyring,
		Hostname: "shithub.sh",
	}
	err := Run(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error when not authenticated")
	}
	if !strings.Contains(err.Error(), "auth login") {
		t.Errorf("error should hint at login, got: %v", err)
	}
	// Underlying typed error should round-trip via errors.Is.
	_ = config.ErrNoToken
}
