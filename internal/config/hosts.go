// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DefaultHost is shithub-cli's canonical target host. Overridable per
// invocation via --hostname or the SHITHUB_HOST env var; per-install via
// hosts.yml's "default: true" marker.
const DefaultHost = "shithub.sh"

// hostsFilePerm is the permission mode hosts.yml must have. Tokens live
// here when the keyring is unavailable; anything looser than 0600 is a
// security boundary violation and Load refuses to read it.
const hostsFilePerm os.FileMode = 0o600

// HostEntry is the per-host record persisted in hosts.yml. Tokens prefer
// the system keyring; the OAuthToken field is populated only when
// InsecureStorage is true or the keyring is unavailable.
type HostEntry struct {
	User            string   `yaml:"user"`
	OAuthToken      string   `yaml:"oauth_token,omitempty"`
	GitProtocol     string   `yaml:"git_protocol,omitempty"`
	AccountUser     string   `yaml:"account_user,omitempty"`
	AccountID       string   `yaml:"account_id,omitempty"`
	InsecureStorage bool     `yaml:"insecure_storage,omitempty"`
	Default         bool     `yaml:"default,omitempty"`
	LastScopes      []string `yaml:"last_scopes,omitempty"`
}

// Hosts is the map host -> *HostEntry. We use a named type so methods can
// hang off it cleanly. yaml.v3 marshals/unmarshals map[string]*HostEntry
// directly; no custom marshaler needed.
type Hosts map[string]*HostEntry

// TokenSource explains where Token resolved the active token from.
// Surfaced in `shithub auth status` so users know whether their token is
// keyring-protected, sitting in a 0600 file, or coming from an env var.
type TokenSource int

// TokenSource values returned by ResolveToken. The zero value is
// TokenSourceUnknown so callers can detect "I forgot to set this" bugs.
const (
	TokenSourceUnknown TokenSource = iota
	TokenSourceEnv
	TokenSourceKeyring
	TokenSourceInsecureFile
)

// String returns a human-readable label suitable for status output.
func (s TokenSource) String() string {
	switch s {
	case TokenSourceEnv:
		return "environment"
	case TokenSourceKeyring:
		return "keyring"
	case TokenSourceInsecureFile:
		return "hosts.yml (insecure)"
	default:
		return "unknown"
	}
}

// ErrHostsFilePerm is returned by LoadHosts when hosts.yml exists but its
// permissions are looser than 0600. We refuse to read tokens out of a
// world-readable file; the user must `chmod 600` before we proceed.
var ErrHostsFilePerm = errors.New("hosts.yml has insecure permissions; run 'chmod 600' on the file")

// LoadHosts reads and parses hosts.yml. A missing file is not an error —
// it just means the user has never logged in; an empty Hosts is returned.
//
// On Unix, file permissions stricter than 0600 are required; looser perms
// return ErrHostsFilePerm so the user repairs them before the file is
// trusted. Windows uses a different ACL model; we skip the check there.
func LoadHosts() (Hosts, error) {
	path, err := HostsFile()
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Hosts{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hosts: stat %s: %w", path, err)
	}

	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			return nil, fmt.Errorf("hosts: %s: %w (got %o)", path, ErrHostsFilePerm, perm)
		}
	}

	raw, err := os.ReadFile(path) //nolint:gosec // path is our own HostsFile()
	if err != nil {
		return nil, fmt.Errorf("hosts: read %s: %w", path, err)
	}

	h := Hosts{}
	if len(raw) == 0 {
		return h, nil
	}
	if err := yaml.Unmarshal(raw, &h); err != nil {
		return nil, fmt.Errorf("hosts: parse %s: %w", path, err)
	}
	return h, nil
}

// Save writes hosts.yml back to disk via an atomic temp-rename with 0600
// perms. An empty Hosts removes the file rather than persisting an empty
// document; round-tripping should converge on "no entries -> no file".
func (h Hosts) Save() error {
	path, err := HostsFile()
	if err != nil {
		return err
	}

	if len(h) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("hosts: remove %s: %w", path, err)
		}
		return nil
	}

	if _, err := EnsureDir(); err != nil {
		return err
	}

	data, err := yaml.Marshal(h)
	if err != nil {
		return fmt.Errorf("hosts: marshal: %w", err)
	}

	return atomicWriteFile(path, data, hostsFilePerm)
}

// Get returns the entry for a host, creating a zero one on miss. The
// returned pointer is owned by Hosts; mutate freely and call Save.
func (h Hosts) Get(host string) *HostEntry {
	host = NormalizeHost(host)
	if e, ok := h[host]; ok {
		return e
	}
	e := &HostEntry{}
	h[host] = e
	return e
}

// Delete removes a host entry. Safe on missing hosts.
func (h Hosts) Delete(host string) {
	delete(h, NormalizeHost(host))
}

// DefaultHostName resolves the active host from the hosts map:
//  1. Any entry marked Default: true wins.
//  2. If exactly one host exists, that one wins.
//  3. Otherwise fall back to the package-level DefaultHost constant.
//
// Env-var and flag overrides (--hostname, SHITHUB_HOST) live in env.go;
// this function only consults the persisted map.
func (h Hosts) DefaultHostName() string {
	if len(h) == 0 {
		return DefaultHost
	}
	for host, e := range h {
		if e != nil && e.Default {
			return host
		}
	}
	if len(h) == 1 {
		for host := range h {
			return host
		}
	}
	return DefaultHost
}

// SetDefault marks `host` as the default and clears the Default flag on
// every other entry. Returns an error if the host has no entry yet —
// callers must Get() (or do a prior login) before setting default.
func (h Hosts) SetDefault(host string) error {
	host = NormalizeHost(host)
	target, ok := h[host]
	if !ok {
		return fmt.Errorf("hosts: cannot mark %q as default: no entry", host)
	}
	for k, e := range h {
		if e == nil {
			continue
		}
		e.Default = k == host
	}
	target.Default = true
	return nil
}

// NormalizeHost trims protocol prefixes and trailing slashes/whitespace
// so callers can pass "https://shithub.sh/", "shithub.sh", or
// "  SHITHUB.SH " interchangeably. Inputs that are clearly NOT a bare
// host — embedded userinfo (`user@host`), a path component after the
// host, an empty label, whitespace, or any character outside the
// LDH+colon+dot set — return the empty string. Callers conventionally
// treat empty as "use the default host," so hostile inputs collapse to
// the safe default rather than carrying their malformed form forward.
// IPv6 bracket form is supported.
func NormalizeHost(host string) string {
	h := strings.TrimSpace(host)
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	h = strings.TrimSuffix(h, "/")
	if h == "" {
		return ""
	}
	if !isValidHost(h) {
		return ""
	}
	return strings.ToLower(h)
}

// isValidHost reports whether h is a bare host[:port] string. We accept
// LDH labels (letters, digits, hyphen) joined by dots, plus an optional
// `:port` suffix, plus the IPv6 bracket form. Anything containing `@`,
// `/`, `?`, `#`, whitespace, or other characters is rejected.
func isValidHost(h string) bool {
	if strings.ContainsAny(h, "@/?# \t\r\n") {
		return false
	}
	// IPv6 bracket form: [::1]:443 — strip the brackets and accept;
	// detailed v6 syntax validation is the OS resolver's job.
	if strings.HasPrefix(h, "[") {
		closeIdx := strings.Index(h, "]")
		if closeIdx < 2 { // "[]" or no closer
			return false
		}
		// Allow optional ":port" tail.
		tail := h[closeIdx+1:]
		if tail != "" && !strings.HasPrefix(tail, ":") {
			return false
		}
		return true
	}
	// Split host and optional port.
	hostPart := h
	if colon := strings.LastIndex(h, ":"); colon >= 0 {
		hostPart = h[:colon]
		port := h[colon+1:]
		if port == "" {
			return false
		}
		for _, r := range port {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	if hostPart == "" {
		return false
	}
	// LDH labels separated by dots; empty labels (leading/trailing/
	// consecutive dot) rejected.
	for _, label := range strings.Split(hostPart, ".") {
		if label == "" {
			return false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= 'A' && r <= 'Z':
			case r >= '0' && r <= '9':
			case r == '-':
			default:
				return false
			}
		}
	}
	return true
}
