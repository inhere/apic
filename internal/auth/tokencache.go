package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const expirySkew = 30 * time.Second

type cacheEntry struct {
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expires_at"` // zero: never expires
	Fingerprint string    `json:"fingerprint"`
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', 0:
			return '_'
		}
		return r
	}, s)
}

func (p *Provider) cachePath() string {
	name := sanitize(p.site) + "." + sanitize(p.envName) + "." + sanitize(p.authName) + ".json"
	return filepath.Join(p.deps.CacheDir, "tokens", name)
}

// fingerprint binds a cached token to base_url and the login definition.
// json.Marshal sorts map keys, so the encoding is stable.
func (p *Provider) fingerprint() string {
	if p.spec.Type == "exec" {
		return p.execFingerprint()
	}
	raw, _ := json.Marshal(p.spec.Login)
	sum := sha256.Sum256([]byte(p.env.BaseURL + "\n" + string(raw)))
	return hex.EncodeToString(sum[:])
}

func (p *Provider) valid(e cacheEntry) bool {
	if e.Token == "" || e.Fingerprint != p.fingerprint() {
		return false
	}
	if p.spec.Type == "exec" && e.ExpiresAt.IsZero() {
		return false // exec tokens are cached only with a known expiry
	}
	return e.ExpiresAt.IsZero() || p.deps.Now().Add(expirySkew).Before(e.ExpiresAt)
}

// readCache returns a usable cached entry, or ok=false.
func (p *Provider) readCache() (cacheEntry, bool) {
	if p.deps.CacheDir == "" {
		return cacheEntry{}, false
	}
	raw, err := os.ReadFile(p.cachePath())
	if err != nil {
		return cacheEntry{}, false
	}
	var e cacheEntry
	if json.Unmarshal(raw, &e) != nil || !p.valid(e) {
		return cacheEntry{}, false
	}
	return e, true
}

func (p *Provider) writeCache(e cacheEntry) error {
	if p.deps.CacheDir == "" {
		return nil
	}
	path := p.cachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tok-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (p *Provider) removeCache() {
	if p.deps.CacheDir != "" {
		_ = os.Remove(p.cachePath())
	}
}

// Logout drops the in-memory and cached token. A missing cache is not an error.
func (p *Provider) Logout() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token, p.expires = "", time.Time{}
	if p.deps.CacheDir == "" {
		return nil
	}
	if err := os.Remove(p.cachePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// StatusInfo describes the cached token of one identity.
type StatusInfo struct {
	LoggedIn  bool
	ExpiresAt time.Time // zero: no expiry known
	Cacheable bool      // false: the token is never cached (exec without expiry or ttl)
}

// Status reports whether a valid cached token exists and when it expires.
func (p *Provider) Status() StatusInfo {
	info := StatusInfo{Cacheable: p.spec.Type != "exec" || p.execCacheable()}
	if e, ok := p.readCache(); ok {
		info.LoggedIn, info.ExpiresAt = true, e.ExpiresAt
	}
	return info
}
