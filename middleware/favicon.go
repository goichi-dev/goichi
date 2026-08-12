package middleware

import (
	"encoding/base64"
	"github.com/goichi-dev/goichi/core"
)

const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="8" fill="#E24B4A"/>
  <text x="16" y="23" font-family="system-ui,sans-serif" font-size="20" font-weight="700"
        text-anchor="middle" fill="#ffffff">G</text>
</svg>`

var faviconBytes []byte

func init() {
	faviconBytes, _ = base64.StdEncoding.DecodeString(
		base64.StdEncoding.EncodeToString([]byte(faviconSVG)),
	)
}

func Favicon() core.Middleware {
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			if string(c.RequestCtx.Path()) == "/favicon.ico" {
				c.RequestCtx.SetContentType("image/svg+xml")
				c.RequestCtx.SetStatusCode(core.StatusOK)
				c.RequestCtx.Response.Header.Set("Cache-Control", "public, max-age=86400")
				c.RequestCtx.SetBody(faviconBytes)
				return nil
			}
			return next(c)
		}
	}
}
