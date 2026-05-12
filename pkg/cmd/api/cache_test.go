// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// withCacheDir points the api cache at a fresh subdirectory of t.TempDir.
func withCacheDir(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "shithub")
	t.Setenv(config.EnvConfigDir, root)
	dir, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	return dir
}

func TestCacheKeyDeterministic(t *testing.T) {
	t.Parallel()
	headers := http.Header{"Accept": []string{"application/json"}}
	body := []byte(`{"x":1}`)

	a := CacheKey("shithub.sh", "GET", "https://shithub.sh/api/v1/user", headers, body)
	b := CacheKey("shithub.sh", "GET", "https://shithub.sh/api/v1/user", headers, body)
	if a != b {
		t.Errorf("same inputs should produce same key: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("key should be sha256 hex (64 chars), got %d", len(a))
	}
}

func TestCacheKeyAuthorizationStripped(t *testing.T) {
	t.Parallel()
	body := []byte(`{}`)
	h1 := http.Header{"Authorization": []string{"token a"}, "Accept": []string{"application/json"}}
	h2 := http.Header{"Authorization": []string{"token b"}, "Accept": []string{"application/json"}}

	if CacheKey("h", "GET", "/u", h1, body) != CacheKey("h", "GET", "/u", h2, body) {
		t.Error("Authorization rotation should not affect cache key")
	}
}

func TestCacheKeyDifferentHostsDiffer(t *testing.T) {
	t.Parallel()
	a := CacheKey("shithub.sh", "GET", "/api/v1/user", nil, nil)
	b := CacheKey("staging.shithub.sh", "GET", "/api/v1/user", nil, nil)
	if a == b {
		t.Error("different hosts should produce different keys")
	}
}

func TestCacheGetMiss(t *testing.T) {
	withCacheDir(t)
	got, hit, err := CacheGet("nonexistent", time.Hour)
	if err != nil {
		t.Fatalf("CacheGet: %v", err)
	}
	if hit || got != nil {
		t.Errorf("miss should yield (nil, false), got (%v, %v)", got, hit)
	}
}

func TestCachePutGetRoundTrip(t *testing.T) {
	withCacheDir(t)
	key := "abcd"
	h := http.Header{"Content-Type": []string{"application/json"}}
	body := []byte(`{"hello":"world"}`)

	if err := CachePut(key, 200, h, body); err != nil {
		t.Fatalf("CachePut: %v", err)
	}
	got, hit, err := CacheGet(key, time.Hour)
	if err != nil {
		t.Fatalf("CacheGet: %v", err)
	}
	if !hit {
		t.Fatal("CacheGet: expected hit")
	}
	if got.Status != 200 {
		t.Errorf("Status: got %d", got.Status)
	}
	if string(got.Body) != string(body) {
		t.Errorf("Body: got %q", got.Body)
	}
	if got.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("Headers preserved? %v", got.Headers)
	}
}

func TestCacheExpiry(t *testing.T) {
	withCacheDir(t)
	key := "expire"
	if err := CachePut(key, 200, nil, []byte(`{}`)); err != nil {
		t.Fatalf("CachePut: %v", err)
	}
	// Backdate the entry by overwriting the file with a past timestamp.
	dir, _ := CacheDir()
	path := filepath.Join(dir, key+".json")
	old := []byte(`{"status":200,"body":"e30=","created_at":"2000-01-01T00:00:00Z"}`)
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, hit, err := CacheGet(key, time.Hour)
	if err != nil {
		t.Fatalf("CacheGet: %v", err)
	}
	if hit {
		t.Error("expired entry should be a miss")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expired entry should be removed; stat err=%v", err)
	}
}

func TestCacheCorruptDiscarded(t *testing.T) {
	withCacheDir(t)
	dir, _ := CacheDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, hit, err := CacheGet("corrupt", time.Hour)
	if err != nil {
		t.Fatalf("CacheGet: %v", err)
	}
	if hit {
		t.Error("corrupt entry should miss, not return")
	}
	// Corrupt file should be removed so subsequent CachePut works cleanly.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("corrupt entry should be removed: stat err=%v", err)
	}
}

func TestIsSensitiveCacheHeader(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"Authorization", "Cookie", "Set-Cookie", "Proxy-Authorization", "Accept-Encoding"} {
		if !isSensitiveCacheHeader(h) {
			t.Errorf("%s should be sensitive", h)
		}
	}
	for _, h := range []string{"Accept", "Content-Type", "X-Request-Id"} {
		if isSensitiveCacheHeader(h) {
			t.Errorf("%s should not be sensitive", h)
		}
	}
}

func TestCacheDirUnderConfigDir(t *testing.T) {
	t.Setenv(config.EnvConfigDir, filepath.Join(t.TempDir(), "sh"))
	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}
	if want := filepath.Join("cache", "api"); !strings.HasSuffix(got, want) {
		t.Errorf("CacheDir path: want suffix %q, got %q", want, got)
	}
}
