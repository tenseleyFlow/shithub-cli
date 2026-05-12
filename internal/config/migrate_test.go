// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import "testing"

func TestMigrateNilReturnsDefault(t *testing.T) {
	t.Parallel()
	c, err := Migrate(nil)
	if err != nil {
		t.Fatalf("Migrate(nil): %v", err)
	}
	if c.Version != SchemaVersion {
		t.Errorf("Version: want %d got %d", SchemaVersion, c.Version)
	}
}

func TestMigrateStampsVersion(t *testing.T) {
	t.Parallel()
	c := &Config{Version: 0, Editor: "vim"}
	got, err := Migrate(c)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if got.Version != SchemaVersion {
		t.Errorf("Version: want %d got %d", SchemaVersion, got.Version)
	}
	if got.Editor != "vim" {
		t.Errorf("Editor should be preserved, got %q", got.Editor)
	}
}
