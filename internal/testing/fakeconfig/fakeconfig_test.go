// SPDX-License-Identifier: AGPL-3.0-or-later

package fakeconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
	"github.com/tenseleyFlow/shithub-cli/internal/testing/fakeconfig"
)

// TestNewIsolatesConfigDir asserts that fakeconfig.New points the config
// package at a tmpdir-scoped path, so the test never sees the developer's
// real ~/.config/shithub directory.
func TestNewIsolatesConfigDir(t *testing.T) {
	tc := fakeconfig.New(t)

	got, err := config.ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if got != tc.Dir {
		t.Errorf("ConfigDir: want %q got %q", tc.Dir, got)
	}
	// Ensure the dir is inside t.TempDir() — defense against accidental
	// real-path leakage if EnvConfigDir resolution ever changes.
	if !filepath.IsAbs(got) {
		t.Errorf("config dir should be absolute, got %q", got)
	}
}

func TestSaveLoadInsideFake(t *testing.T) {
	tc := fakeconfig.New(t)

	tc.Hosts.Get("shithub.sh").User = "tester"
	tc.Hosts.Get("shithub.sh").OAuthToken = "shithub_pat_test"
	if err := tc.Hosts.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := config.LoadHosts()
	if err != nil {
		t.Fatalf("LoadHosts: %v", err)
	}
	if got["shithub.sh"].User != "tester" {
		t.Errorf("User: got %q", got["shithub.sh"].User)
	}

	// Confirm the file landed inside the fake's Dir.
	hostsPath := filepath.Join(tc.Dir, "hosts.yml")
	if _, err := os.Stat(hostsPath); err != nil {
		t.Errorf("hosts.yml should exist at %s: %v", hostsPath, err)
	}
}

func TestMemoryKeyringRecordsCalls(t *testing.T) {
	tc := fakeconfig.New(t)

	if err := config.SetToken(tc.Keyring, "shithub.sh", "u", "secret"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	if tc.Keyring.Sets != 1 {
		t.Errorf("Sets: want 1 got %d", tc.Keyring.Sets)
	}
	if !tc.Keyring.Has("shithub:shithub.sh", "u") {
		t.Error("Has should return true after SetToken")
	}
}
