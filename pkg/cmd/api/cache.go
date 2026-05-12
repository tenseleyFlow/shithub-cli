// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/tenseleyFlow/shithub-cli/internal/config"
)

// cachedResponse is the serialized form on disk. Status, headers, and
// body together let replay reconstruct the response faithfully; CreatedAt
// powers TTL expiry without parsing the file modification time (which
// can drift across filesystems).
type cachedResponse struct {
	Status    int         `json:"status"`
	Headers   http.Header `json:"headers"`
	Body      []byte      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
}

// cacheKeyComponents are the inputs hashed into the cache key. We hash a
// JSON-encoded representation so reordering or formatting changes don't
// silently break key compatibility. Authorization is deliberately
// excluded so token rotation does not invalidate every cached entry;
// the host distinguishes per-server caches.
type cacheKeyComponents struct {
	Host    string      `json:"host"`
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Headers http.Header `json:"headers,omitempty"`
	Body    []byte      `json:"body,omitempty"`
}

// CacheKey returns the disk-safe key for the given request. Headers are
// passed through a copy because we strip Authorization in place.
func CacheKey(host, method, urlStr string, headers http.Header, body []byte) string {
	clean := http.Header{}
	for k, v := range headers {
		if isSensitiveCacheHeader(k) {
			continue
		}
		// Keep deterministic order by sorting the value slice.
		cv := append([]string(nil), v...)
		sort.Strings(cv)
		clean[k] = cv
	}
	comps := cacheKeyComponents{
		Host:    host,
		Method:  method,
		URL:     urlStr,
		Headers: clean,
		Body:    body,
	}
	enc, _ := json.Marshal(comps)
	sum := sha256.Sum256(enc)
	return hex.EncodeToString(sum[:])
}

// isSensitiveCacheHeader names headers we strip from the cache-key
// hash. Authorization rotates with token churn; Cookie carries session
// state; Accept-Encoding is transport, not identity.
func isSensitiveCacheHeader(name string) bool {
	switch name {
	case "Authorization", "Cookie", "Set-Cookie", "Proxy-Authorization", "Accept-Encoding":
		return true
	}
	return false
}

// CacheDir resolves the on-disk path for API response cache entries.
// Subdirectory keeps it adjacent to (but isolated from) any future
// per-feature cache.
func CacheDir() (string, error) {
	base, err := config.CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "api"), nil
}

// CacheGet returns the cached response for key if it exists and has not
// expired. Returns (nil, false, nil) on a miss; an error only on I/O
// failures, never on an expected miss or a malformed (corrupt) entry —
// corrupt entries are silently discarded so a single bad file doesn't
// block the whole cache.
func CacheGet(key string, ttl time.Duration) (*cachedResponse, bool, error) {
	dir, err := CacheDir()
	if err != nil {
		return nil, false, err
	}
	path := filepath.Join(dir, key+".json")
	raw, err := os.ReadFile(path) //nolint:gosec // key is sha256 hex; path is package-controlled
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache: read %s: %w", path, err)
	}
	var entry cachedResponse
	if err := json.Unmarshal(raw, &entry); err != nil {
		_ = os.Remove(path) // corrupt; recover gracefully
		return nil, false, nil
	}
	if ttl > 0 && time.Since(entry.CreatedAt) > ttl {
		// Expired; remove eagerly to keep the dir bounded.
		_ = os.Remove(path)
		return nil, false, nil
	}
	return &entry, true, nil
}

// CachePut writes the response to disk atomically. Errors propagate but
// are non-fatal at the call site (callers proceed with the live response
// when caching fails). Directory creation uses 0700 perms because the
// cache may contain non-public response bodies.
func CachePut(key string, status int, headers http.Header, body []byte) error {
	dir, err := CacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cache: mkdir %s: %w", dir, err)
	}
	entry := cachedResponse{
		Status:    status,
		Headers:   headers,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, key+".json")
	return atomicWrite(path, encoded, 0o600)
}

// atomicWrite stages the bytes in a same-directory temp file then
// renames into place, mirroring config.atomicWriteFile. We don't pull
// that helper across package boundaries because the api package would
// pick up an unwanted dependency on internal/config for one function.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
