package middleware

import (
	"github.com/goichi-dev/goichi/core"
	"github.com/valyala/fasthttp"
)

func Compress() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			if err := next(c); err != nil {
				return err
			}

			ae := string(c.RequestCtx.Request.Header.Peek("Accept-Encoding"))
			body := c.RequestCtx.Response.Body()

			if len(body) < 1024 {
				return nil
			}

			if contains(ae, "gzip") {
				compressed := fasthttp.AppendGzipBytes(nil, body)
				c.RequestCtx.Response.Header.Set("Content-Encoding", "gzip")
				c.RequestCtx.Response.Header.Set("Vary", "Accept-Encoding")
				c.RequestCtx.Response.SetBody(compressed)
			} else if contains(ae, "deflate") {
				compressed := fasthttp.AppendDeflateBytes(nil, body)
				c.RequestCtx.Response.Header.Set("Content-Encoding", "deflate")
				c.RequestCtx.Response.Header.Set("Vary", "Accept-Encoding")
				c.RequestCtx.Response.SetBody(compressed)
			}

			return nil
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub ||
		len(s) > 0 && findSubstr(s, sub))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
