package middleware

import (
	"fmt"
	"github.com/goichi-dev/goichi/core"
)

func BodyLimit(maxBytes int) core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			if len(c.RequestCtx.Request.Body()) > maxBytes {
				c.RequestCtx.SetStatusCode(core.StatusRequestEntityTooLarge)
				c.RequestCtx.SetContentType("application/json")
				c.RequestCtx.SetBodyString(fmt.Sprintf(`{"error":"request body exceeds limit of %s"}`, formatBytes(maxBytes)))
				return nil
			}
			return next(c)
		}
	}
}

func formatBytes(b int) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%d MB", b/1024/1024)
	case b >= 1024:
		return fmt.Sprintf("%d KB", b/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
