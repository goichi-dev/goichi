package middleware

import (
	"github.com/goichi-dev/goichi/core"
	"sync"
	"time"
)

type cacheEntry struct {
	body        []byte
	contentType string
	status      int
	expiresAt   time.Time
}

type cacheStore struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

func newCacheStore() *cacheStore {
	s := &cacheStore{entries: make(map[string]*cacheEntry)}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.evict()
		}
	}()
	return s
}

func (s *cacheStore) get(key string) (*cacheEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e, true
}

func (s *cacheStore) set(key string, e *cacheEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = e
}

func (s *cacheStore) evict() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.entries {
		if now.After(e.expiresAt) {
			delete(s.entries, k)
		}
	}
}

type CacheConfig struct {
	TTL     time.Duration
	KeyFunc func(c *core.Context) string
	Methods []string
}

var defaultCacheMethods = []string{"GET", "HEAD"}

func Cache(cfg CacheConfig) core.Middleware {
	if cfg.TTL == 0 {
		cfg.TTL = 30 * time.Second
	}
	if len(cfg.Methods) == 0 {
		cfg.Methods = defaultCacheMethods
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = func(c *core.Context) string {
			return string(c.RequestCtx.Method()) + ":" +
				string(c.RequestCtx.Path()) + "?" +
				string(c.RequestCtx.QueryArgs().QueryString())
		}
	}

	store := newCacheStore()
	allowed := make(map[string]bool, len(cfg.Methods))
	for _, m := range cfg.Methods {
		allowed[m] = true
	}

	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			if !allowed[string(c.RequestCtx.Method())] {
				return next(c)
			}

			// Do not cache authenticated responses by default
			if len(c.RequestCtx.Request.Header.Peek("Authorization")) > 0 {
				return next(c)
			}

			key := cfg.KeyFunc(c)

			if entry, ok := store.get(key); ok {
				c.RequestCtx.Response.SetStatusCode(entry.status)
				c.RequestCtx.Response.Header.Set("Content-Type", entry.contentType)
				c.RequestCtx.Response.Header.Set("X-Cache", "HIT")
				c.RequestCtx.Response.SetBody(entry.body)
				return nil
			}

			c.RequestCtx.Response.Header.Set("X-Cache", "MISS")
			if err := next(c); err != nil {
				return err
			}

			status := c.RequestCtx.Response.StatusCode()
			if status >= 200 && status < 300 {
				body := make([]byte, len(c.RequestCtx.Response.Body()))
				copy(body, c.RequestCtx.Response.Body())
				store.set(key, &cacheEntry{
					body:        body,
					contentType: string(c.RequestCtx.Response.Header.ContentType()),
					status:      status,
					expiresAt:   time.Now().Add(cfg.TTL),
				})
			}

			return nil
		}
	}
}
