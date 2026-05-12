// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"testing"
)

// clearTokenEnv unsets every env var ResolveToken consults, so each test
// starts from a known-clean baseline regardless of the developer's shell.
func clearTokenEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvToken,
		EnvEnterpriseToken,
		EnvAcceptGithubToken,
		EnvGithubToken,
		EnvGHToken,
		EnvHost,
	} {
		t.Setenv(k, "")
	}
}

func TestResolveTokenEnvWinsForDefaultHost(t *testing.T) {
	clearTokenEnv(t)
	t.Setenv(EnvToken, "shithub_pat_envtoken")

	ks := newFakeKeyring()
	h := Hosts{
		"shithub.sh": {User: "u", OAuthToken: "shithub_pat_file"},
	}
	tok, src, err := ResolveToken(ks, h, "shithub.sh", "shithub.sh")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if tok != "shithub_pat_envtoken" {
		t.Errorf("token: want envtoken, got %q", tok)
	}
	if src != TokenSourceEnv {
		t.Errorf("source: want env, got %v", src)
	}
}

func TestResolveTokenEnterpriseEnvForNonDefaultHost(t *testing.T) {
	clearTokenEnv(t)
	t.Setenv(EnvToken, "shithub_pat_default")
	t.Setenv(EnvEnterpriseToken, "shithub_pat_enterprise")

	tok, src, err := ResolveToken(nil, Hosts{}, "shithub.sh", "internal.example.com")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if tok != "shithub_pat_enterprise" {
		t.Errorf("non-default host should use ENTERPRISE_TOKEN, got %q", tok)
	}
	if src != TokenSourceEnv {
		t.Errorf("source: want env, got %v", src)
	}
}

func TestResolveTokenGithubTokenOptIn(t *testing.T) {
	t.Run("off by default", func(t *testing.T) {
		clearTokenEnv(t)
		t.Setenv(EnvGithubToken, "ghp_abc")

		_, _, err := ResolveToken(nil, Hosts{}, "shithub.sh", "shithub.sh")
		if !errors.Is(err, ErrNoToken) {
			t.Errorf("GITHUB_TOKEN should be ignored without opt-in, got: %v", err)
		}
	})

	t.Run("honored with SHITHUB_ACCEPT_GITHUB_TOKEN=1", func(t *testing.T) {
		clearTokenEnv(t)
		t.Setenv(EnvAcceptGithubToken, "1")
		t.Setenv(EnvGithubToken, "ghp_abc")

		tok, src, err := ResolveToken(nil, Hosts{}, "shithub.sh", "shithub.sh")
		if err != nil {
			t.Fatalf("ResolveToken: %v", err)
		}
		if tok != "ghp_abc" {
			t.Errorf("token: want ghp_abc, got %q", tok)
		}
		if src != TokenSourceEnv {
			t.Errorf("source: want env, got %v", src)
		}
	})

	t.Run("GH_TOKEN preferred over GITHUB_TOKEN", func(t *testing.T) {
		clearTokenEnv(t)
		t.Setenv(EnvAcceptGithubToken, "1")
		t.Setenv(EnvGHToken, "ghp_gh")
		t.Setenv(EnvGithubToken, "ghp_github")

		tok, _, err := ResolveToken(nil, Hosts{}, "shithub.sh", "shithub.sh")
		if err != nil {
			t.Fatalf("ResolveToken: %v", err)
		}
		if tok != "ghp_gh" {
			t.Errorf("want GH_TOKEN to win, got %q", tok)
		}
	})

	t.Run("github tokens not honored for non-default host", func(t *testing.T) {
		clearTokenEnv(t)
		t.Setenv(EnvAcceptGithubToken, "1")
		t.Setenv(EnvGithubToken, "ghp_abc")

		_, _, err := ResolveToken(nil, Hosts{}, "shithub.sh", "internal.example")
		if !errors.Is(err, ErrNoToken) {
			t.Errorf("non-default host should not honor GH_TOKEN, got %v", err)
		}
	})
}

func TestResolveTokenKeyringBeforeInsecureFile(t *testing.T) {
	clearTokenEnv(t)

	ks := newFakeKeyring()
	_ = SetToken(ks, "shithub.sh", "u", "shithub_pat_keyring")

	h := Hosts{
		"shithub.sh": {User: "u", OAuthToken: "shithub_pat_file"},
	}
	tok, src, err := ResolveToken(ks, h, "shithub.sh", "shithub.sh")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if tok != "shithub_pat_keyring" {
		t.Errorf("keyring should win over file, got %q", tok)
	}
	if src != TokenSourceKeyring {
		t.Errorf("source: want keyring, got %v", src)
	}
}

func TestResolveTokenInsecureFileWhenKeyringEmpty(t *testing.T) {
	clearTokenEnv(t)

	ks := newFakeKeyring()
	h := Hosts{
		"shithub.sh": {User: "u", OAuthToken: "shithub_pat_file", InsecureStorage: true},
	}
	tok, src, err := ResolveToken(ks, h, "shithub.sh", "shithub.sh")
	if err != nil {
		t.Fatalf("ResolveToken: %v", err)
	}
	if tok != "shithub_pat_file" {
		t.Errorf("file should win when keyring empty, got %q", tok)
	}
	if src != TokenSourceInsecureFile {
		t.Errorf("source: want insecure file, got %v", src)
	}
}

func TestResolveTokenUnknownHost(t *testing.T) {
	clearTokenEnv(t)
	_, _, err := ResolveToken(nil, Hosts{}, "shithub.sh", "missing.example")
	if !errors.Is(err, ErrNoToken) {
		t.Errorf("want ErrNoToken, got %v", err)
	}
}

func TestResolveHostFlagWins(t *testing.T) {
	clearTokenEnv(t)
	t.Setenv(EnvHost, "env.example")
	h := Hosts{"persisted.example": {User: "u", Default: true}}

	got := ResolveHost("flag.example", h)
	if got != "flag.example" {
		t.Errorf("flag should win, got %q", got)
	}
}

func TestResolveHostEnvBeforePersisted(t *testing.T) {
	clearTokenEnv(t)
	t.Setenv(EnvHost, "env.example")
	h := Hosts{"persisted.example": {User: "u", Default: true}}

	got := ResolveHost("", h)
	if got != "env.example" {
		t.Errorf("env should beat persisted, got %q", got)
	}
}

func TestResolveHostFallbackToPersistedDefault(t *testing.T) {
	clearTokenEnv(t)
	h := Hosts{"persisted.example": {User: "u", Default: true}}

	got := ResolveHost("", h)
	if got != "persisted.example" {
		t.Errorf("persisted default should win when no flag/env, got %q", got)
	}
}

func TestResolveHostUltimateFallback(t *testing.T) {
	clearTokenEnv(t)
	if got := ResolveHost("", Hosts{}); got != DefaultHost {
		t.Errorf("want %q, got %q", DefaultHost, got)
	}
	if got := ResolveHost("", nil); got != DefaultHost {
		t.Errorf("want %q on nil hosts, got %q", DefaultHost, got)
	}
}
