package proxy

import (
	"fmt"
	"github.com/goichi-dev/goichi/core"
	"github.com/valyala/fasthttp"
	"strings"
	"time"
)

type Config struct {
	// Target is the base URL of the upstream server (e.g., "http://localhost:8080")
	Target string
	// StripPrefix is the part of the path to remove before forwarding (e.g., "/api/v1")
	StripPrefix string
	// Timeout is the maximum time to wait for the upstream response
	Timeout time.Duration
	// ModifyRequest allows custom modification of the request before sending
	ModifyRequest func(c *core.Context, req *fasthttp.Request)
	// ModifyResponse allows custom modification of the response after receiving
	ModifyResponse func(c *core.Context, res *fasthttp.Response)
}

func (c *Config) SetDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Target != "" && !strings.HasPrefix(c.Target, "http") {
		c.Target = "http://" + c.Target
	}
	c.Target = strings.TrimSuffix(c.Target, "/")
}

func New(cfg Config) core.Middleware {
	cfg.SetDefaults()

	client := &fasthttp.Client{
		ReadTimeout:  cfg.Timeout,
		WriteTimeout: cfg.Timeout,
	}

	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			req := &c.RequestCtx.Request
			res := &c.RequestCtx.Response

			// 1. Prepare Request Path
			originalPath := string(req.URI().Path())
			proxyPath := originalPath
			if cfg.StripPrefix != "" {
				proxyPath = strings.TrimPrefix(proxyPath, cfg.StripPrefix)
				if proxyPath == "" {
					proxyPath = "/"
				}
				if !strings.HasPrefix(proxyPath, "/") {
					proxyPath = "/" + proxyPath
				}
			}

			// 2. Set Upstream URL
			newURL := cfg.Target + proxyPath
			if query := req.URI().QueryString(); len(query) > 0 {
				newURL += "?" + string(query)
			}
			req.SetRequestURI(newURL)

			// 3. Header Forwarding (Standard Proxy Headers)
			req.Header.Set("X-Forwarded-For", c.RequestCtx.RemoteIP().String())
			req.Header.Set("X-Forwarded-Host", string(req.Host()))
			req.Header.Set("X-Forwarded-Proto", "http") // default for now

			// 4. Custom Modification before sending
			if cfg.ModifyRequest != nil {
				cfg.ModifyRequest(c, req)
			}

			// 5. Do Request
			if err := client.Do(req, res); err != nil {
				c.RequestCtx.SetStatusCode(fasthttp.StatusBadGateway)
				c.RequestCtx.SetBodyString(fmt.Sprintf(`{"error":"proxy error: %v"}`, err))
				return nil
			}

			// 6. Custom Modification after receiving
			if cfg.ModifyResponse != nil {
				cfg.ModifyResponse(c, res)
			}

			return nil
		}
	}
}
