// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"os"
	"strings"
)

// lookupEnvTrim is os.LookupEnv with a TrimSpace pass on the value.
// We trim because shell users commonly type `SHITHUB_TOKEN=" "` (a
// single space) intending an explicit-empty override; treating that
// as "no token" matches the all-whitespace intent. Returns (trimmed
// value, true) when the var was set in the environment.
func lookupEnvTrim(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(v), true
}

// Env-var names for token + host resolution. Centralized so callers
// import symbols, not strings; renaming a var here is a one-grep change.
const (
	// EnvToken is consulted first for the default host. Mirrors gh's
	// GH_TOKEN env-var role.
	EnvToken = "SHITHUB_TOKEN"

	// EnvEnterpriseToken is consulted for non-default (self-hosted) shithub
	// instances, mirroring gh's GH_ENTERPRISE_TOKEN. Useful in CI where the
	// active host is a self-hosted shithub and the default-host token would
	// be incorrect.
	EnvEnterpriseToken = "SHITHUB_ENTERPRISE_TOKEN"

	// EnvHost overrides the resolved active host. Lower priority than the
	// --hostname flag but higher than the persisted hosts.yml default.
	EnvHost = "SHITHUB_HOST"

	// EnvAcceptGithubToken opts the user into honoring GITHUB_TOKEN /
	// GH_TOKEN for shithub. Off by default to prevent accidental cross-host
	// token leakage; opt-in semantics match gh's reverse case.
	EnvAcceptGithubToken = "SHITHUB_ACCEPT_GITHUB_TOKEN"
	EnvGithubToken       = "GITHUB_TOKEN"
	EnvGHToken           = "GH_TOKEN"
)

// ErrNoToken is returned by ResolveToken when no token is found anywhere
// in the precedence chain. Commands should surface this with a hint like
// "run `shithub auth login` to authenticate".
var ErrNoToken = errors.New("no token configured for host")

// ResolveToken returns the active token for `host` plus its source.
//
// Precedence:
//  1. SHITHUB_TOKEN              (when host == default host).
//  2. SHITHUB_ENTERPRISE_TOKEN   (when host != default host).
//  3. GITHUB_TOKEN / GH_TOKEN    (only if SHITHUB_ACCEPT_GITHUB_TOKEN=1
//     AND host == default host).
//  4. System keyring             (via the supplied KeyringStore).
//  5. hosts.yml insecure_storage field.
//
// `defaultHost` is the host the env-var chain should match against; pass
// the result of Hosts.DefaultHostName().
func ResolveToken(ks KeyringStore, h Hosts, defaultHost, host string) (string, TokenSource, error) {
	host = NormalizeHost(host)
	defaultHost = NormalizeHost(defaultHost)

	// H9: distinguish "env unset" from "env set to empty string". The
	// latter is an explicit "I want no token" override (useful in CI
	// or while reproducing an unauthenticated state); pre-fix it
	// silently fell through to the keyring. We surface it as a
	// dedicated source so `auth status` doesn't lie about where the
	// (lack of) token came from.
	if host == defaultHost {
		if tok, ok := lookupEnvTrim(EnvToken); ok {
			if tok == "" {
				return "", TokenSourceEnvEmpty, ErrNoToken
			}
			return tok, TokenSourceEnv, nil
		}
		if os.Getenv(EnvAcceptGithubToken) == "1" {
			if tok, ok := lookupEnvTrim(EnvGHToken); ok {
				if tok == "" {
					return "", TokenSourceEnvEmpty, ErrNoToken
				}
				return tok, TokenSourceEnv, nil
			}
			if tok, ok := lookupEnvTrim(EnvGithubToken); ok {
				if tok == "" {
					return "", TokenSourceEnvEmpty, ErrNoToken
				}
				return tok, TokenSourceEnv, nil
			}
		}
	} else {
		if tok, ok := lookupEnvTrim(EnvEnterpriseToken); ok {
			if tok == "" {
				return "", TokenSourceEnvEmpty, ErrNoToken
			}
			return tok, TokenSourceEnv, nil
		}
	}

	entry, ok := h[host]
	if !ok || entry == nil {
		return "", TokenSourceUnknown, ErrNoToken
	}

	if !entry.InsecureStorage && entry.User != "" && ks != nil {
		tok, err := GetToken(ks, host, entry.User)
		if err == nil && tok != "" {
			return tok, TokenSourceKeyring, nil
		}
		// Fall through to InsecureFile path; ErrKeyringNotFound is the common
		// case for a host configured via --insecure-storage and is not fatal.
	}

	if entry.OAuthToken != "" {
		return entry.OAuthToken, TokenSourceInsecureFile, nil
	}

	return "", TokenSourceUnknown, ErrNoToken
}

// ResolveHost picks the active host using the precedence chain:
//  1. `flagHost` (set by --hostname; empty when the flag was not supplied).
//  2. SHITHUB_HOST env var.
//  3. Hosts.DefaultHostName() (entry marked default, or single entry, or
//     the package-level DefaultHost constant).
func ResolveHost(flagHost string, h Hosts) string {
	if flagHost != "" {
		return NormalizeHost(flagHost)
	}
	if env := os.Getenv(EnvHost); env != "" {
		return NormalizeHost(env)
	}
	if h == nil {
		return DefaultHost
	}
	return h.DefaultHostName()
}
