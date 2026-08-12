package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/goichi-dev/goichi/core"
)

func RequestID() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			id := c.Header("X-Request-ID")
			if id == "" {
				id = newRequestID()
			}

			c.RequestCtx.Request.Header.Set("X-Request-ID", id)
			c.RequestCtx.Response.Header.Set("X-Request-ID", id)

			return next(c)
		}
	}
}

func newRequestID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}
