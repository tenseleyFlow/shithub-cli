// SPDX-License-Identifier: AGPL-3.0-or-later

// Package fakeconfig builds an isolated Config + Hosts + Keyring trio
// rooted in a t.TempDir() so command-level tests never touch the user's
// real config directory or system keyring. Pull in via:
//
//	tc := fakeconfig.New(t)
//	tc.Hosts.Get("shithub.sh").User = "tester"
//	// ... drive the unit under test against tc.Keyring / tc.Hosts ...
package fakeconfig

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// TestConfig bundles a fake keyring and an isolated config directory.
// Every field is populated and ready to use after New(); callers mutate
// Hosts and call Hosts.Save() to persist within the temp dir.
type TestConfig struct {
	Dir     string         // resolved config directory inside t.TempDir()
	Keyring *MemoryKeyring // in-memory keyring; no OS interaction
	Hosts   config.Hosts   // empty by default; mutate + Save()
	Config  *config.Config // Default() seed; mutate + Save()
}

// New points the config package at a fresh subdirectory of t.TempDir()
// and returns a populated TestConfig. The cleanup is automatic: the
// SHITHUB_CONFIG_DIR env var is restored when the test ends (via
// t.Setenv) and t.TempDir() is removed by the testing framework.
func New(t *testing.T) *TestConfig {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "shithub")
	t.Setenv(config.EnvConfigDir, dir)
	// Defensive: clear inherited env so resolution starts clean.
	for _, k := range []string{
		config.EnvToken,
		config.EnvEnterpriseToken,
		config.EnvAcceptGithubToken,
		config.EnvGithubToken,
		config.EnvGHToken,
		config.EnvHost,
	} {
		t.Setenv(k, "")
	}

	return &TestConfig{
		Dir:     dir,
		Keyring: NewMemoryKeyring(),
		Hosts:   config.Hosts{},
		Config:  config.Default(),
	}
}

// MemoryKeyring is an in-memory implementation of config.KeyringStore for
// tests. It records every operation so assertions can verify the (service,
// account) keys we hit — useful when proving "the token never went to
// the keyring" for an --insecure-storage path.
type MemoryKeyring struct {
	mu    sync.Mutex
	store map[string]string // "service\x00account" -> secret
	// Set/Get/Delete counters help tests assert call patterns without
	// reaching into the store directly.
	Sets    int
	Gets    int
	Deletes int
}

// NewMemoryKeyring returns a ready-to-use empty keyring.
func NewMemoryKeyring() *MemoryKeyring {
	return &MemoryKeyring{store: map[string]string{}}
}

func (m *MemoryKeyring) key(service, account string) string {
	return service + "\x00" + account
}

// Set records a secret in memory.
func (m *MemoryKeyring) Set(service, account, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sets++
	m.store[m.key(service, account)] = secret
	return nil
}

// Get returns a previously-set secret, or keyring.ErrNotFound if absent.
func (m *MemoryKeyring) Get(service, account string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Gets++
	v, ok := m.store[m.key(service, account)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

// Delete removes an entry. Missing entries return keyring.ErrNotFound so
// the wrapping config.DeleteToken can normalize to a no-op.
func (m *MemoryKeyring) Delete(service, account string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Deletes++
	if _, ok := m.store[m.key(service, account)]; !ok {
		return keyring.ErrNotFound
	}
	delete(m.store, m.key(service, account))
	return nil
}

// Has is a test-only helper that reports whether a secret exists, without
// affecting the Get counter. Useful for `if !kr.Has(...) { t.Error(...) }`.
func (m *MemoryKeyring) Has(service, account string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.store[m.key(service, account)]
	return ok
}
