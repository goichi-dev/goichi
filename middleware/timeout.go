package middleware

import (
	"github.com/goichi-dev/goichi/core"
	"github.com/valyala/fasthttp"
	"time"
)

// Timeout aborts a request with 503 if the handler runs longer than d.
//
// fasthttp reuses *RequestCtx across requests, so a handler left running after
// a timeout could race (or corrupt) a later request if it kept writing to the
// shared context. To make the timeout race-free, the handler runs against an
// isolated context built from a *copy* of the request and writes into its own
// response. Only if the handler finishes first do we copy that response back to
// the real client; on timeout the detached work is simply discarded.
func Timeout(d time.Duration) core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			type result struct {
				err error
				sub *fasthttp.RequestCtx
			}
			done := make(chan result, 1)

			// Snapshot the request so the goroutine never touches the shared ctx.
			var reqCopy fasthttp.Request
			c.RequestCtx.Request.CopyTo(&reqCopy)

			subCtx := *c // shallow copy: Params/Locals/JWTClaims are not read by the timeout path
			go func() {
				var sub fasthttp.RequestCtx
				sub.Init(&reqCopy, c.RequestCtx.RemoteAddr(), nil)
				subCtx.RequestCtx = &sub
				err := next(&subCtx)
				done <- result{err: err, sub: &sub}
			}()

			select {
			case r := <-done:
				// Handler won: publish its response to the real context.
				r.sub.Response.CopyTo(&c.RequestCtx.Response)
				return r.err
			case <-time.After(d):
				return c.Status(core.StatusServiceUnavailable).
					JSON(map[string]string{"error": "request timeout"})
			}
		}
	}
}
