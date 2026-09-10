package threads

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"
)

// Cache is a simple on-disk blob cache keyed by request URL with an mtime TTL.
type Cache struct {
	dir     string
	enabled bool
	ttl     time.Duration
}

// NewCache returns a cache rooted at dir. When enabled is false every operation
// is a no-op.
func NewCache(dir string, enabled bool, ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &Cache{dir: dir, enabled: enabled, ttl: ttl}
}

func (c *Cache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	h := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, h[:2], h)
}

// Get returns a cached body if present and fresh.
func (c *Cache) Get(key string) ([]byte, bool) {
	if !c.enabled {
		return nil, false
	}
	p := c.path(key)
	fi, err := os.Stat(p)
	if err != nil {
		return nil, false
	}
	if time.Since(fi.ModTime()) > c.ttl {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	return b, true
}

// Put stores a body. Failures are silent: the cache is best-effort.
func (c *Cache) Put(key string, body []byte) {
	if !c.enabled {
		return
	}
	p := c.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(p, body, 0o644)
}

// Clear removes the whole cache directory.
func (c *Cache) Clear() error { return os.RemoveAll(c.dir) }

// Dir returns the cache root.
func (c *Cache) Dir() string { return c.dir }
