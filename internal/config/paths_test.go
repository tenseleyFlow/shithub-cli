// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

// TestConfigDirPrecedence locks the precedence: SHITHUB_CONFIG_DIR over
// XDG over HOME-derived. Uses t.Setenv so it must run sequentially.
func TestConfigDirPrecedence(t *testing.T) {
	const (
		shithubOverride = "/tmp/sh-test-override"
		xdg             = "/tmp/sh-test-xdg"
		home            = "/tmp/sh-test-home"
		winAppData      = "C:\\Users\\test\\AppData"
	)

	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "SHITHUB_CONFIG_DIR wins over everything",
			env: map[string]string{
				EnvConfigDir:     shithubOverride,
				EnvXDGConfigHome: xdg,
				EnvHome:          home,
			},
			want: shithubOverride,
		},
		{
			name: "XDG_CONFIG_HOME wins over HOME",
			env: map[string]string{
				EnvConfigDir:     "",
				EnvXDGConfigHome: xdg,
				EnvHome:          home,
			},
			want: filepath.Join(xdg, "shithub"),
		},
	}

	if runtime.GOOS == "windows" {
		cases = append(cases, struct {
			name string
			env  map[string]string
			want string
		}{
			name: "Windows uses AppData when XDG unset",
			env: map[string]string{
				EnvConfigDir:      "",
				EnvXDGConfigHome:  "",
				EnvWindowsAppData: winAppData,
			},
			want: filepath.Join(winAppData, "shithub"),
		})
	} else {
		cases = append(cases, struct {
			name string
			env  map[string]string
			want string
		}{
			name: "Unix HOME fallback",
			env: map[string]string{
				EnvConfigDir:     "",
				EnvXDGConfigHome: "",
				EnvHome:          home,
			},
			want: filepath.Join(home, ".config", "shithub"),
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			got, err := ConfigDir()
			if err != nil {
				t.Fatalf("ConfigDir: %v", err)
			}
			if got != tc.want {
				t.Errorf("ConfigDir: want %q got %q", tc.want, got)
			}
		})
	}
}

// TestConfigDirHomeUnset asserts a clear error when nothing in the
// fallback chain is available.
func TestConfigDirHomeUnset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses a different fallback chain")
	}
	t.Setenv(EnvConfigDir, "")
	t.Setenv(EnvXDGConfigHome, "")
	t.Setenv(EnvHome, "")
	_, err := ConfigDir()
	if err == nil {
		t.Fatal("expected error when HOME is unset")
	}
}

// TestPathsRelativeToConfigDir verifies ConfigFile, HostsFile, CacheDir
// all derive from the same root.
func TestPathsRelativeToConfigDir(t *testing.T) {
	t.Setenv(EnvConfigDir, "/tmp/sh-test")

	cfg, err := ConfigFile()
	if err != nil {
		t.Fatalf("ConfigFile: %v", err)
	}
	if cfg != "/tmp/sh-test/config.yml" {
		t.Errorf("ConfigFile: got %q", cfg)
	}

	hosts, err := HostsFile()
	if err != nil {
		t.Fatalf("HostsFile: %v", err)
	}
	if hosts != "/tmp/sh-test/hosts.yml" {
		t.Errorf("HostsFile: got %q", hosts)
	}

	cache, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if cache != "/tmp/sh-test/cache" {
		t.Errorf("CacheDir: got %q", cache)
	}
}

// TestEnsureDirCreates0700 verifies EnsureDir creates the dir with 0700
// permissions on Unix. Windows permission model is different; skip there.
func TestEnsureDirCreates0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits don't translate on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "shithub-test")
	t.Setenv(EnvConfigDir, target)

	got, err := EnsureDir()
	if err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if got != target {
		t.Errorf("EnsureDir path: want %q got %q", target, got)
	}

	info, err := stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("perm: want 0700 got %o", perm)
	}
}
