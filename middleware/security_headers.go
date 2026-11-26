package middleware

import "github.com/goichi-dev/goichi/core"

var securityHeaders = [...][2][]byte{
	{[]byte("X-Content-Type-Options"), []byte("nosniff")},
	{[]byte("X-Frame-Options"), []byte("DENY")},
	{[]byte("Referrer-Policy"), []byte("no-referrer")},
	{[]byte("X-Xss-Protection"), []byte("0")},
	{[]byte("Content-Security-Policy"), []byte("default-src 'self'; script-src 'self' cdn.jsdelivr.net; style-src 'self' cdn.jsdelivr.net 'unsafe-inline';")},
	{[]byte("Strict-Transport-Security"), []byte("max-age=63072000; includeSubDomains; preload")},
}

// SecurityHeaders adds common HTTP security headers. Enable in production.
func SecurityHeaders() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			for _, h := range securityHeaders {
				c.RequestCtx.Response.Header.SetCanonical(h[0], h[1])
			}
			return next(c)
		}
	}
}
