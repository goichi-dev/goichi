package middleware

import (
	"github.com/goichi-dev/goichi/core"
	"log"
	"sync/atomic"
	"time"
)

var (
	totalRequests  uint64
	activeRequests int64
)

// Metrics returns a middleware that tracks basic request statistics
func Metrics() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			atomic.AddUint64(&totalRequests, 1)
			atomic.AddInt64(&activeRequests, 1)
			defer atomic.AddInt64(&activeRequests, -1)

			return next(c)
		}
	}
}

// GetStats returns the current global request statistics
func GetStats() (total uint64, active int64) {
	return atomic.LoadUint64(&totalRequests), atomic.LoadInt64(&activeRequests)
}

// StatsLogger periodically logs the current stats
func StatsLogger(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		for range ticker.C {
			total, active := GetStats()
			log.Printf("[Metrics] Total Requests: %d, Active: %d", total, active)
		}
	}()
}
