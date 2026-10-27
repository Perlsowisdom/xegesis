package apibible

import (
	"sync"
	"time"
)

// Cache terms: API.Bible requires cached content to be refreshed at least every 30 days and
// recommends 14 days or less. The cache lives in memory only, so nothing outlives the app.
const (
	MaxCacheAge     = 30 * 24 * time.Hour
	DefaultCacheAge = 14 * 24 * time.Hour
)

type entry struct {
	body []byte
	at   time.Time
}

// Cache holds raw responses by request URL (the API key is never part of the key or the value).
type Cache struct {
	mu  sync.Mutex
	ttl time.Duration
	now func() time.Time
	m   map[string]entry
}

// NewCache makes a cache. A ttl outside (0, 30 days] is replaced by the default, so the cache can
// never hold content longer than the terms allow.
func NewCache(ttl time.Duration) *Cache {
	if ttl <= 0 || ttl > MaxCacheAge {
		ttl = DefaultCacheAge
	}
	return &Cache{ttl: ttl, now: time.Now, m: map[string]entry{}}
}

func (c *Cache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return nil, false
	}
	if c.now().Sub(e.at) >= c.ttl {
		delete(c.m, key)
		return nil, false
	}
	return e.body, true
}

func (c *Cache) put(key string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = entry{body: body, at: c.now()}
}

// Clear drops everything, for example when the user changes keys.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m = map[string]entry{}
}

// Len is the number of cached responses.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}
