// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// TestSaveAtomicNeverCorruptsFile covers audit #152: concurrent Save
// calls (the cross-process or future cross-goroutine scenario) race on
// hosts.yml, but the atomic temp-rename means the surviving file is
// always one of the writers' inputs intact — never a torn half-write
// or a YAML-unparseable mess. We spam N goroutines saving distinct
// host sets, then load the file and verify it parses cleanly.
func TestSaveAtomicNeverCorruptsFile(t *testing.T) {
	if testing.Short() {
		t.Skip("concurrency stress test")
	}
	withConfigDir(t)

	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			h := Hosts{}
			h.Get(fmt.Sprintf("h%d.test", i)).User = fmt.Sprintf("u%d", i)
			_ = h.Save() // races are expected; we only care about file validity
		}(i)
	}
	wg.Wait()

	// LoadHosts must succeed — atomic rename guarantees the survivor is
	// internally consistent even when many writers raced.
	got, err := LoadHosts()
	if err != nil {
		t.Fatalf("file is corrupt after concurrent Save: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected exactly one surviving host entry; got %d (%v)", len(got), got)
	}
}

// TestValidateHostStrict covers the audit #158 regression: callers that
// need to distinguish "user typed garbage" from "user typed nothing" go
// through ValidateHost. Empty input errors with a hostname-required
// message; malformed input errors with the actual value in the message
// so the user can see what was rejected.
func TestValidateHostStrict(t *testing.T) {
	t.Parallel()
	// Empty → required-error.
	if _, err := ValidateHost(""); err == nil {
		t.Error("empty input should error")
	} else if !strings.Contains(err.Error(), "required") {
		t.Errorf("error should say required, got: %v", err)
	}
	if _, err := ValidateHost("   "); err == nil {
		t.Error("whitespace-only input should error")
	}
	// Malformed → invalid-error with the rejected input quoted.
	bad := []string{"attacker@victim", "host/path", "foo space", "foo..bar"}
	for _, in := range bad {
		if _, err := ValidateHost(in); err == nil {
			t.Errorf("malformed %q should error", in)
		} else if !strings.Contains(err.Error(), in) {
			t.Errorf("error should echo input %q; got: %v", in, err)
		}
	}
	// Valid → normalized string + nil.
	for _, in := range []string{"shithub.sh", "Shithub.SH:8443", " host.local "} {
		got, err := ValidateHost(in)
		if err != nil {
			t.Errorf("ValidateHost(%q) errored: %v", in, err)
			continue
		}
		if got == "" {
			t.Errorf("ValidateHost(%q) returned empty", in)
		}
	}
}

// TestValidateHostRefusesPlaintextScheme pins H17: pre-fix
// `--hostname http://shithub.sh` was silently coerced to
// `shithub.sh` and any subsequent request was issued over HTTPS,
// but the typo signals plaintext intent — and `shithub api
// http://host/...` (related) would actually leak the bearer token
// on the first hop. Refuse the explicit http:// prefix.
func TestValidateHostRefusesPlaintextScheme(t *testing.T) {
	for _, in := range []string{
		"http://shithub.sh",
		"HTTP://Shithub.SH",
		"  http://shithub.sh  ",
	} {
		if _, err := ValidateHost(in); err == nil {
			t.Errorf("expected refusal for %q", in)
		} else if !strings.Contains(err.Error(), "plaintext") {
			t.Errorf("error should call out plaintext for %q: %v", in, err)
		}
	}
	// https:// stays accepted — NormalizeHost strips the prefix.
	if _, err := ValidateHost("https://shithub.sh"); err != nil {
		t.Errorf("https should be accepted: %v", err)
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
		"shithub.sh:8443":     "shithub.sh:8443",
		"[::1]":               "[::1]",
		"[::1]:8443":          "[::1]:8443",
		// C-audit C24: explicit `:443` (HTTPS default) is normalized
		// off so users pasting Host headers from HTTP traces hit the
		// hosts.yml entry. Non-default ports must survive.
		"shithub.sh:443":         "shithub.sh",
		"https://shithub.sh:443": "shithub.sh",
		"SHITHUB.SH:443":         "shithub.sh",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("%q -> want %q got %q", in, want, got)
		}
	}
}

// TestNormalizeHostRejectsMalformed covers the audit #138 tightening:
// inputs that aren't a bare host[:port] return the empty string so the
// caller's "use default" fallback kicks in rather than carrying a
// hostile form (userinfo, path component, internal whitespace) into
// later URL composition.
func TestNormalizeHostRejectsMalformed(t *testing.T) {
	bad := []string{
		"attacker@victim",
		"https://attacker@victim/",
		"shithub.sh/path",
		"https://shithub.sh/owner/repo",
		"shithub.sh path",
		"shi\nthub.sh", // internal newline (TrimSpace only touches edges)
		"shithub..sh",
		".shithub.sh",
		"shithub.sh.",
		"shithub.sh:",
		"shithub.sh:abc",
		"sh@hub.sh:443",
		"foo#bar",
		"foo?baz",
		"[::1",      // unclosed bracket
		"[::1]junk", // tail without leading colon
		"😀.example",
		"",
	}
	for _, in := range bad {
		if got := NormalizeHost(in); got != "" {
			t.Errorf("expected empty rejection for %q; got %q", in, got)
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
