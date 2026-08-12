package middleware

import (
	"github.com/goichi-dev/goichi/core"
	"strings"
)

// CORSConfig controls cross-origin access. No origin is allowed until
// AllowOrigins is set.
type CORSConfig struct {
	AllowOrigins []string
	AllowMethods []string
	AllowHeaders []string
	MaxAge       int
}

// DefaultCORSConfig allows the common methods and headers but no origin.
var DefaultCORSConfig = CORSConfig{
	AllowOrigins: []string{}, // require explicit configuration in production
	AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
	AllowHeaders: []string{"Origin", "Content-Type", "Authorization", "Accept"},
	MaxAge:       86400,
}

// CORS answers preflight requests and adds the access-control headers. The
// router dispatches OPTIONS to this middleware automatically, so preflight works
// without declaring OPTIONS routes.
func CORS(cfg ...CORSConfig) core.Middleware {
	c := DefaultCORSConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}

	methods := strings.Join(c.AllowMethods, ", ")
	headers := strings.Join(c.AllowHeaders, ", ")

	return func(next core.Handler) core.Handler {
		return func(ctx *core.Context) error {
			origin := string(ctx.RequestCtx.Request.Header.Peek("Origin"))

			allowed := ""
			for _, o := range c.AllowOrigins {
				if o == "*" || o == origin {
					allowed = o
					break
				}
			}

			// Responses vary by Origin whenever the allowlist is origin-specific,
			// so shared caches must not serve one origin's response to another.
			ctx.RequestCtx.Response.Header.Add("Vary", "Origin")

			if allowed != "" {
				ctx.RequestCtx.Response.Header.Set("Access-Control-Allow-Origin", allowed)
				ctx.RequestCtx.Response.Header.Set("Access-Control-Allow-Methods", methods)
				ctx.RequestCtx.Response.Header.Set("Access-Control-Allow-Headers", headers)

				if c.MaxAge > 0 {
					ctx.RequestCtx.Response.Header.Set("Access-Control-Max-Age",
						strings.Join([]string{itoa(c.MaxAge)}, ""))
				}
			}

			if string(ctx.RequestCtx.Method()) == "OPTIONS" {
				ctx.RequestCtx.SetStatusCode(core.StatusNoContent)
				return nil
			}

			return next(ctx)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte(n%10) + '0'
		n /= 10
	}
	return string(buf[pos:])
}
