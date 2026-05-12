// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadHostsMissingReturnsEmpty(t *testing.T) {
	withConfigDir(t)

	h, err := LoadHosts()
	if err != nil {
		t.Fatalf("LoadHosts: %v", err)
	}
	if len(h) != 0 {
		t.Errorf("want empty Hosts, got %d entries", len(h))
	}
}

func TestSaveLoadHostsRoundTrip(t *testing.T) {
	withConfigDir(t)

	src := Hosts{}
	src.Get("shithub.sh").User = "mfwolffe"
	src.Get("shithub.sh").OAuthToken = "shithub_pat_abcdef"
	src.Get("shithub.sh").GitProtocol = "ssh"
	src.Get("shithub.sh").Default = true
	src.Get("staging.shithub.sh").User = "mfwolffe-staging"
	src.Get("staging.shithub.sh").InsecureStorage = true

	if err := src.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadHosts()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got["shithub.sh"].User != "mfwolffe" {
		t.Errorf("User: got %q", got["shithub.sh"].User)
	}
	if got["shithub.sh"].OAuthToken != "shithub_pat_abcdef" {
		t.Errorf("OAuthToken: got %q", got["shithub.sh"].OAuthToken)
	}
	if !got["shithub.sh"].Default {
		t.Error("Default flag lost in round trip")
	}
	if !got["staging.shithub.sh"].InsecureStorage {
		t.Error("InsecureStorage flag lost in round trip")
	}
}

func TestSaveHostsPerm0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ACL model differs on Windows")
	}
	dir := withConfigDir(t)

	h := Hosts{}
	h.Get("shithub.sh").User = "u"
	h.Get("shithub.sh").OAuthToken = "shithub_pat_xxx"
	if err := h.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "hosts.yml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("hosts.yml perms: want 0600 got %o", perm)
	}
}

func TestLoadHostsRefusesLoosePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ACL model differs on Windows")
	}
	dir := withConfigDir(t)
	if _, err := EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	path := filepath.Join(dir, "hosts.yml")
	if err := os.WriteFile(path, []byte("shithub.sh:\n  user: u\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, err := LoadHosts()
	if err == nil {
		t.Fatal("expected ErrHostsFilePerm, got nil")
	}
	if !errors.Is(err, ErrHostsFilePerm) {
		t.Errorf("want ErrHostsFilePerm, got %v", err)
	}
}

func TestSaveEmptyHostsRemovesFile(t *testing.T) {
	dir := withConfigDir(t)

	h := Hosts{}
	h.Get("shithub.sh").User = "u"
	if err := h.Save(); err != nil {
		t.Fatalf("initial Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hosts.yml")); err != nil {
		t.Fatalf("file should exist after first Save: %v", err)
	}

	empty := Hosts{}
	if err := empty.Save(); err != nil {
		t.Fatalf("empty Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hosts.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file should be gone after empty Save, stat err: %v", err)
	}
}

func TestDefaultHostNameResolution(t *testing.T) {
	cases := []struct {
		name  string
		hosts Hosts
		want  string
	}{
		{
			name:  "empty falls back to package default",
			hosts: Hosts{},
			want:  DefaultHost,
		},
		{
			name: "single entry wins",
			hosts: Hosts{
				"only.example.com": {User: "u"},
			},
			want: "only.example.com",
		},
		{
			name: "explicit default wins over count",
			hosts: Hosts{
				"shithub.sh":         {User: "u"},
				"staging.shithub.sh": {User: "u", Default: true},
				"internal.example":   {User: "u"},
			},
			want: "staging.shithub.sh",
		},
		{
			name: "multiple entries without default fall back",
			hosts: Hosts{
				"a.example": {User: "u"},
				"b.example": {User: "u"},
			},
			want: DefaultHost,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.hosts.DefaultHostName(); got != tc.want {
				t.Errorf("want %q got %q", tc.want, got)
			}
		})
	}
}

func TestSetDefault(t *testing.T) {
	h := Hosts{}
	h.Get("a.example").Default = true
	h.Get("b.example")
	h.Get("c.example")

	if err := h.SetDefault("b.example"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if h["a.example"].Default {
		t.Error("a.example should no longer be default")
	}
	if !h["b.example"].Default {
		t.Error("b.example should be default")
	}
}

func TestSetDefaultUnknownHost(t *testing.T) {
	h := Hosts{}
	h.Get("a.example")
	if err := h.SetDefault("missing.example"); err == nil {
		t.Error("expected error for unknown host")
	}
}

func TestNormalizeHost(t *testing.T) {
	cases := map[string]string{
		"shithub.sh":          "shithub.sh",
		"  shithub.sh  ":      "shithub.sh",
		"https://shithub.sh":  "shithub.sh",
		"http://shithub.sh/":  "shithub.sh",
		"SHITHUB.SH":          "shithub.sh",
		"https://SHITHUB.SH/": "shithub.sh",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("%q -> want %q got %q", in, want, got)
		}
	}
}

func TestTokenSourceString(t *testing.T) {
	cases := map[TokenSource]string{
		TokenSourceEnv:          "environment",
		TokenSourceKeyring:      "keyring",
		TokenSourceInsecureFile: "hosts.yml (insecure)",
		TokenSourceUnknown:      "unknown",
	}
	for src, want := range cases {
		if got := src.String(); got != want {
			t.Errorf("%d -> want %q got %q", src, want, got)
		}
	}
	// Containment sanity: hosts.yml source must not include the raw token.
	if strings.Contains(TokenSourceInsecureFile.String(), "shithub_pat_") {
		t.Error("source label should never embed a token")
	}
}
