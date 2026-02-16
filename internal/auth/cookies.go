// Package auth handles Twitch authentication, cookie persistence, and
// credential management for the miner.
package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cookie represents a single HTTP cookie persisted to JSON.
type Cookie struct {
	Name string `json:"name"`
	Value string `json:"value"`
	Domain string `json:"domain,omitempty"`
	Path string `json:"path,omitempty"`
	Expires time.Time `json:"expires,omitempty"`
}

// CookieJar manages a collection of cookies with thread-safe access
// and JSON persistence. It replaces the Python pickle-based cookie storage.
type CookieJar struct {
	mu      sync.RWMutex
	cookies []Cookie
}

// NewCookieJar creates an empty CookieJar.
func NewCookieJar() *CookieJar {
	return &CookieJar{
		cookies: make([]Cookie, 0),
	}
}

// Load reads cookies from a JSON file at the given path.
// Returns an error if the file does not exist or cannot be parsed.
func (cj *CookieJar) Load(path string) error {
	cj.mu.Lock()
	defer cj.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading cookie file %s: %w", path, err)
	}

	var cookies []Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return fmt.Errorf("parsing cookie file %s: %w", path, err)
	}

	cj.cookies = cookies
	return nil
}

// Save writes the current cookies to a JSON file at the given path.
// It creates parent directories if they do not exist.
// Uses atomic write (write to temp file, then rename) to prevent corruption.
func (cj *CookieJar) Save(path string) error {
	cj.mu.RLock()
	defer cj.mu.RUnlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating cookie directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cj.cookies, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cookies: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("writing temp cookie file %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp cookie file %s to %s: %w", tmpPath, path, err)
	}

	return nil
}

// Get returns the value of a cookie by name, or empty string if not found.
func (cj *CookieJar) Get(name string) string {
	cj.mu.RLock()
	defer cj.mu.RUnlock()

	for _, c := range cj.cookies {
		if c.Name == name && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

// Set adds or updates a cookie by name.
func (cj *CookieJar) Set(name, value string) {
	cj.mu.Lock()
	defer cj.mu.Unlock()

	for i, c := range cj.cookies {
		if c.Name == name {
			cj.cookies[i].Value = value
			return
		}
	}
	cj.cookies = append(cj.cookies, Cookie{
		Name:   name,
		Value:  value,
		Domain: ".twitch.tv",
		Path:   "/",
	})
}

// All returns a copy of all cookies.
func (cj *CookieJar) All() []Cookie {
	cj.mu.RLock()
	defer cj.mu.RUnlock()

	result := make([]Cookie, len(cj.cookies))
	copy(result, cj.cookies)
	return result
}

// Len returns the number of cookies in the jar.
func (cj *CookieJar) Len() int {
	cj.mu.RLock()
	defer cj.mu.RUnlock()
	return len(cj.cookies)
}

// Clear removes all cookies from the jar.
func (cj *CookieJar) Clear() {
	cj.mu.Lock()
	defer cj.mu.Unlock()
	cj.cookies = make([]Cookie, 0)
}

// CookieFileExists checks if a cookie file exists at the given path.
func CookieFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
