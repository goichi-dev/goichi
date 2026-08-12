package middleware

import "github.com/goichi-dev/goichi/core"

// SecurityHeaders adds common HTTP security headers. Enable in production.
func SecurityHeaders() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			// Use Response.Header.Set directly to avoid copying header value
			c.RequestCtx.Response.Header.Set("X-Content-Type-Options", "nosniff")
			c.RequestCtx.Response.Header.Set("X-Frame-Options", "DENY")
			c.RequestCtx.Response.Header.Set("Referrer-Policy", "no-referrer")
			c.RequestCtx.Response.Header.Set("X-XSS-Protection", "0")
			c.RequestCtx.Response.Header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' cdn.jsdelivr.net; style-src 'self' cdn.jsdelivr.net 'unsafe-inline';")
			c.RequestCtx.Response.Header.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			return next(c)
		}
	}
}
