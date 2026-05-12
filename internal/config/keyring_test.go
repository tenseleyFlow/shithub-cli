// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

// fakeKeyring is an in-memory KeyringStore for tests. It records every
// call so assertions can verify we hit the right (service, account) keys.
type fakeKeyring struct {
	mu            sync.Mutex
	store         map[string]string // "service\x00account" -> secret
	failSet       bool
	failGet       bool
	failDelete    bool
	notFoundOnGet bool
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{store: map[string]string{}}
}

func (f *fakeKeyring) key(service, account string) string {
	return service + "\x00" + account
}

func (f *fakeKeyring) Set(service, account, secret string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSet {
		return errors.New("fake: set failed")
	}
	f.store[f.key(service, account)] = secret
	return nil
}

func (f *fakeKeyring) Get(service, account string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failGet {
		return "", errors.New("fake: get failed")
	}
	if f.notFoundOnGet {
		return "", keyring.ErrNotFound
	}
	v, ok := f.store[f.key(service, account)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (f *fakeKeyring) Delete(service, account string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDelete {
		return errors.New("fake: delete failed")
	}
	if _, ok := f.store[f.key(service, account)]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.store, f.key(service, account))
	return nil
}

func TestKeyringSvcName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"shithub.sh":          "shithub:shithub.sh",
		"STAGING.SHITHUB.SH":  "shithub:staging.shithub.sh",
		"https://shithub.sh/": "shithub:shithub.sh",
	}
	for in, want := range cases {
		if got := keyringSvcName(in); got != want {
			t.Errorf("%q -> want %q got %q", in, want, got)
		}
	}
}

func TestSetGetDeleteRoundTrip(t *testing.T) {
	t.Parallel()
	ks := newFakeKeyring()

	if err := SetToken(ks, "shithub.sh", "mfwolffe", "shithub_pat_abc"); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	tok, err := GetToken(ks, "shithub.sh", "mfwolffe")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok != "shithub_pat_abc" {
		t.Errorf("token: want shithub_pat_abc got %q", tok)
	}

	if err := DeleteToken(ks, "shithub.sh", "mfwolffe"); err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}

	if _, err := GetToken(ks, "shithub.sh", "mfwolffe"); !errors.Is(err, ErrKeyringNotFound) {
		t.Errorf("after delete: want ErrKeyringNotFound, got %v", err)
	}
}

func TestDeleteTokenIdempotent(t *testing.T) {
	t.Parallel()
	ks := newFakeKeyring()
	if err := DeleteToken(ks, "shithub.sh", "mfwolffe"); err != nil {
		t.Errorf("Delete on absent entry should be a no-op, got: %v", err)
	}
}

func TestGetTokenNotFoundReturnsTyped(t *testing.T) {
	t.Parallel()
	ks := newFakeKeyring()
	ks.notFoundOnGet = true
	_, err := GetToken(ks, "shithub.sh", "anyone")
	if !errors.Is(err, ErrKeyringNotFound) {
		t.Errorf("want ErrKeyringNotFound, got %v", err)
	}
}

func TestSetTokenRejectsEmptyUser(t *testing.T) {
	t.Parallel()
	ks := newFakeKeyring()
	if err := SetToken(ks, "shithub.sh", "", "tok"); err == nil {
		t.Error("expected error on empty user")
	}
}

func TestNilKeyringRejected(t *testing.T) {
	t.Parallel()
	if err := SetToken(nil, "h", "u", "t"); err == nil {
		t.Error("nil keyring should error on Set")
	}
	if _, err := GetToken(nil, "h", "u"); err == nil {
		t.Error("nil keyring should error on Get")
	}
	if err := DeleteToken(nil, "h", "u"); err == nil {
		t.Error("nil keyring should error on Delete")
	}
}

func TestKeyringAvailableTrueWhenSetSucceeds(t *testing.T) {
	t.Parallel()
	ks := newFakeKeyring()
	if !KeyringAvailable(ks) {
		t.Error("KeyringAvailable should be true for working store")
	}
}

func TestKeyringAvailableFalseWhenSetFails(t *testing.T) {
	t.Parallel()
	ks := newFakeKeyring()
	ks.failSet = true
	if KeyringAvailable(ks) {
		t.Error("KeyringAvailable should be false when Set fails")
	}
}

// TestNoTokenLeakInError verifies that even on failure modes we never
// echo the secret back through an error string. Important: a panic dump
// or stderr emission must not include the PAT.
func TestNoTokenLeakInError(t *testing.T) {
	t.Parallel()
	ks := newFakeKeyring()
	ks.failSet = true
	const secret = "shithub_pat_nonexistent_secret"
	err := SetToken(ks, "shithub.sh", "u", secret)
	if err == nil {
		t.Fatal("expected error from failing store")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("token leaked into error message: %v", err)
	}
}
