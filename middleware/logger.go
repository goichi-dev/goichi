package middleware

import (
	"fmt"
	"github.com/goichi-dev/goichi/core"
	"log"
	"time"
)

func Logger() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			start := time.Now()
			err := next(c)
			dur := time.Since(start)

			status := c.RequestCtx.Response.StatusCode()
			method := string(c.RequestCtx.Method())
			path := string(c.RequestCtx.Path())

			color := statusColor(status)
			reset := "\033[0m"

			log.Printf("[GoICHI] %-6s %-30s %s%d%s  %s",
				method, path, color, status, reset, formatDur(dur))

			return err
		}
	}
}

func statusColor(code int) string {
	switch {
	case code >= 500:
		return "\033[31m" // red
	case code >= 400:
		return "\033[33m" // yellow
	case code >= 300:
		return "\033[36m" // cyan
	default:
		return "\033[32m" // green
	}
}

func formatDur(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.2fµs", float64(d.Microseconds()))
	}
	return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000)
}
