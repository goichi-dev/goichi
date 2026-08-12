package middleware

import (
	"github.com/goichi-dev/goichi/core"
	"sync"
	"time"
)

type visitor struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

var visitors sync.Map

func getVisitor(ip string, window time.Duration) *visitor {
	now := time.Now()
	v, _ := visitors.LoadOrStore(ip, &visitor{
		count:   0,
		resetAt: now.Add(window),
	})
	return v.(*visitor)
}

func RateLimit(limit int, window time.Duration) core.Middleware {
	go func() {
		ticker := time.NewTicker(window * 2)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			visitors.Range(func(key, val any) bool {
				v := val.(*visitor)
				v.mu.Lock()
				expired := now.After(v.resetAt)
				v.mu.Unlock()
				if expired {
					visitors.Delete(key)
				}
				return true
			})
		}
	}()

	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			ip := c.RequestCtx.RemoteIP().String()
			v := getVisitor(ip, window)

			v.mu.Lock()
			now := time.Now()
			if now.After(v.resetAt) {
				v.count = 0
				v.resetAt = now.Add(window)
			}
			v.count++
			over := v.count > limit
			v.mu.Unlock()

			if over {
				return c.Status(core.StatusTooManyRequests).JSON(map[string]string{"error": "too many requests, slow down"})
			}
			return next(c)
		}
	}
}
