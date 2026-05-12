// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// KeyringService is the prefix under which we register secrets in the OS
// keyring. The full service name is "shithub:<host>" so multi-host setups
// don't collide. The shape is intentionally close to gh's so a user with
// both binaries installed can tell at a glance which secret belongs to which.
const KeyringService = "shithub"

// KeyringStore abstracts the keyring operations the rest of the package
// uses. The default implementation calls zalando/go-keyring directly; tests
// inject a fake that records calls and never touches the real OS keyring.
type KeyringStore interface {
	Set(service, account, secret string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

// systemKeyring is the production implementation backed by go-keyring.
// It is unexported; callers obtain it via NewSystemKeyring().
type systemKeyring struct{}

func (systemKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (systemKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (systemKeyring) Delete(service, account string) error {
	return keyring.Delete(service, account)
}

// NewSystemKeyring returns the live system keyring backend. Used in
// production paths; tests use a FakeKeyring instead.
func NewSystemKeyring() KeyringStore {
	return systemKeyring{}
}

// keyringSvcName builds the service identifier for a given host.
// Centralized so we never typo the format string at a call site.
func keyringSvcName(host string) string {
	return fmt.Sprintf("%s:%s", KeyringService, NormalizeHost(host))
}

// SetToken writes the token into the OS keyring keyed by (host, user).
// Returns an error if the keyring is unreachable; the caller decides
// whether to fall back to insecure file storage.
func SetToken(ks KeyringStore, host, user, token string) error {
	if ks == nil {
		return errors.New("keyring: nil store")
	}
	if user == "" {
		return errors.New("keyring: empty user")
	}
	return ks.Set(keyringSvcName(host), user, token)
}

// GetToken reads the token from the OS keyring. ErrKeyringNotFound is
// returned when the secret is absent (no token previously stored);
// any other error means the keyring itself is unreachable.
func GetToken(ks KeyringStore, host, user string) (string, error) {
	if ks == nil {
		return "", errors.New("keyring: nil store")
	}
	if user == "" {
		return "", errors.New("keyring: empty user")
	}
	tok, err := ks.Get(keyringSvcName(host), user)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrKeyringNotFound
	}
	return tok, err
}

// DeleteToken removes the keyring entry. Missing entries are not an error
// (idempotent — logout should succeed even if the token is already gone).
func DeleteToken(ks KeyringStore, host, user string) error {
	if ks == nil {
		return errors.New("keyring: nil store")
	}
	if user == "" {
		return errors.New("keyring: empty user")
	}
	err := ks.Delete(keyringSvcName(host), user)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// ErrKeyringNotFound is returned by GetToken when no secret exists for the
// given (host, user). Callers should treat this as "not authenticated"
// and prompt for login, not as an error to surface to the user.
var ErrKeyringNotFound = errors.New("keyring: no token stored for host/user")

// KeyringAvailable returns true if a probe write/read against the system
// keyring succeeds. Used by `auth login` to decide whether to fall back
// to InsecureStorage automatically (e.g., on headless Linux with no
// DBus/Secret-Service). The probe uses a unique account name so we don't
// step on a real token.
func KeyringAvailable(ks KeyringStore) bool {
	const probeAccount = "_shithub-cli-probe"
	const probeHost = "_probe"
	if err := ks.Set(keyringSvcName(probeHost), probeAccount, "probe"); err != nil {
		return false
	}
	// Best-effort cleanup; we don't care if it fails (next probe overwrites).
	_ = ks.Delete(keyringSvcName(probeHost), probeAccount)
	return true
}
