// Package proxy provides reverse proxying for goichi: an HTTP reverse proxy
// mounted as middleware, and a raw TCP proxy that fronts any byte-stream
// service (Postgres, MySQL, Redis, an SMTP relay) as a protocol server.
package proxy

import (
	"crypto/tls"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/goichi-dev/goichi/core"
	"github.com/valyala/fasthttp"
)

// ErrNoTarget is returned when every target is unhealthy.
var ErrNoTarget = errors.New("proxy: no healthy target available")

// hopByHopHeaders are connection-scoped and must not be forwarded upstream or
// back to the client (RFC 9110 7.6.1). Connection itself is handled separately
// because it also names further headers to drop.
var hopByHopHeaders = []string{
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// Config configures an HTTP reverse proxy.
type Config struct {
	// Target is the base URL of a single upstream ("http://localhost:8080").
	// Use Targets instead to balance across several.
	Target string
	// Targets is the upstream pool. When set, Target is ignored.
	Targets []*Target
	// Balancer selects a target per request. Default RoundRobin.
	Balancer Algorithm
	// StripPrefix is removed from the path before forwarding ("/api/v1").
	StripPrefix string
	// AddPrefix is prepended to the path after StripPrefix is applied.
	AddPrefix string
	// Host overrides the Host header sent upstream. Empty forwards the
	// inbound Host, which is what a virtual-hosted backend expects.
	Host string
	// Timeout is the deadline for one upstream request. Default 30s.
	Timeout time.Duration
	// MaxRetries is how many other targets are tried after a transport
	// failure. Default 0.
	MaxRetries int
	// MaxResponseBodySize caps the upstream response body in bytes. Zero uses
	// the fasthttp default.
	MaxResponseBodySize int
	// HealthCheck configures background probing of the target pool.
	HealthCheck HealthCheck
	// TLSConfig is used for https upstreams.
	TLSConfig *tls.Config
	// WebSocket enables transparent passthrough of Upgrade requests: the
	// connection is hijacked and bytes are piped both ways.
	WebSocket bool
	// ModifyRequest can adjust the outgoing request before it is sent.
	ModifyRequest func(c *core.Context, req *fasthttp.Request)
	// ModifyResponse can adjust the upstream response before it is returned.
	ModifyResponse func(c *core.Context, res *fasthttp.Response)
	// ErrorHandler replaces the default 502 when no upstream can be reached.
	ErrorHandler func(c *core.Context, err error)
}

// SetDefaults fills in the timeout and normalizes the configured targets.
func (c *Config) SetDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Target != "" {
		c.Target = normalizeURL(c.Target)
	}
	for _, t := range c.Targets {
		t.URL = normalizeURL(t.URL)
	}
	c.StripPrefix = strings.TrimSuffix(c.StripPrefix, "/")
	c.AddPrefix = strings.TrimSuffix(c.AddPrefix, "/")
}

func normalizeURL(u string) string {
	if u != "" && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "http://" + u
	}
	return strings.TrimSuffix(u, "/")
}

// Proxy is a configured HTTP reverse proxy. Use New to mount one as
// middleware, or NewProxy to own its lifecycle and call Do from a handler.
type Proxy struct {
	cfg     Config
	client  *fasthttp.Client
	bal     *Balancer
	checker *checker
	once    sync.Once
}

// NewProxy returns a Proxy for cfg. Call Start to begin health checking.
func NewProxy(cfg Config) *Proxy {
	cfg.SetDefaults()

	targets := cfg.Targets
	if len(targets) == 0 && cfg.Target != "" {
		targets = []*Target{{URL: cfg.Target}}
	}

	client := &fasthttp.Client{
		ReadTimeout:         cfg.Timeout,
		WriteTimeout:        cfg.Timeout,
		MaxResponseBodySize: cfg.MaxResponseBodySize,
		TLSConfig:           cfg.TLSConfig,
		// The proxy rewrites the request URI itself, so fasthttp must not
		// normalize the path again and change what the client sent.
		DisablePathNormalizing: true,
	}

	p := &Proxy{
		cfg:    cfg,
		client: client,
		bal:    NewBalancer(targets, cfg.Balancer),
	}
	p.checker = newChecker(cfg.HealthCheck, p.bal, httpProbe(cfg.HealthCheck, client))
	return p
}

// Start begins background health checking. It is safe to call more than once.
func (p *Proxy) Start() { p.once.Do(p.checker.start) }

// Stop ends background health checking.
func (p *Proxy) Stop() { p.checker.stop() }

// Targets returns the upstream pool, so callers can report health.
func (p *Proxy) Targets() []*Target { return p.bal.Targets() }

// New returns middleware that forwards every request it handles upstream. It
// never calls next: mount it on a route or group the proxy owns.
func New(cfg Config) core.Middleware {
	p := NewProxy(cfg)
	p.Start()
	return func(next core.Handler) core.Handler {
		return func(c *core.Context) error {
			return p.Do(c)
		}
	}
}

// Balance is New with an explicit target pool, matching the shape most
// load-balancer configurations want.
func Balance(targets []string, cfg Config) core.Middleware {
	pool := make([]*Target, 0, len(targets))
	for _, t := range targets {
		pool = append(pool, &Target{URL: t})
	}
	cfg.Targets = pool
	return New(cfg)
}

// Do forwards one request to an upstream and writes the response into c.
func (p *Proxy) Do(c *core.Context) error {
	ctx := c.RequestCtx

	if p.cfg.WebSocket && isUpgrade(&ctx.Request) {
		return p.doUpgrade(c)
	}

	req := fasthttp.AcquireRequest()
	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(res)

	// Copy rather than mutate the inbound request: it is still needed for
	// logging and error handling after the proxy returns, and hop-by-hop
	// headers must be stripped from the copy only.
	ctx.Request.CopyTo(req)
	stripRequestHopByHop(&req.Header)
	p.setForwardHeaders(c, req)

	path := p.rewritePath(string(ctx.Path()))
	query := ctx.URI().QueryString()

	tried := make(map[*Target]bool)
	attempts := p.cfg.MaxRetries + 1
	var lastErr error

	for i := 0; i < attempts; i++ {
		target := p.pick(c, tried)
		if target == nil {
			break
		}
		tried[target] = true

		uri := target.URL + path
		if len(query) > 0 {
			uri += "?" + string(query)
		}
		req.SetRequestURI(uri)
		if p.cfg.Host != "" {
			req.SetHost(p.cfg.Host)
		}

		if p.cfg.ModifyRequest != nil {
			p.cfg.ModifyRequest(c, req)
		}

		target.active.Add(1)
		err := p.client.DoTimeout(req, res, p.cfg.Timeout)
		target.active.Add(-1)

		if err == nil {
			stripResponseHopByHop(&res.Header)
			res.CopyTo(&ctx.Response)
			if p.cfg.ModifyResponse != nil {
				p.cfg.ModifyResponse(c, &ctx.Response)
			}
			return nil
		}

		lastErr = err
		res.Reset()
	}

	if lastErr == nil {
		lastErr = ErrNoTarget
	}
	return p.fail(c, lastErr)
}

// pick chooses a healthy target that has not been tried for this request.
func (p *Proxy) pick(c *core.Context, tried map[*Target]bool) *Target {
	pool := p.bal.Targets()
	key := c.RequestCtx.RemoteIP().String()

	// Round robin advances on every Pick, so ask repeatedly until an untried
	// target comes up, bounded by the pool size.
	for i := 0; i <= len(pool); i++ {
		t := p.bal.Pick(key)
		if t == nil {
			return nil
		}
		if !tried[t] {
			return t
		}
	}
	return nil
}

func (p *Proxy) fail(c *core.Context, err error) error {
	if p.cfg.ErrorHandler != nil {
		p.cfg.ErrorHandler(c, err)
		return nil
	}
	c.RequestCtx.SetStatusCode(fasthttp.StatusBadGateway)
	return core.WriteJSONError(c.RequestCtx, "proxy: "+err.Error())
}

// rewritePath applies StripPrefix and AddPrefix to the inbound path.
func (p *Proxy) rewritePath(path string) string {
	if p.cfg.StripPrefix != "" {
		path = strings.TrimPrefix(path, p.cfg.StripPrefix)
	}
	if p.cfg.AddPrefix != "" {
		path = p.cfg.AddPrefix + path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// setForwardHeaders adds the standard forwarding headers. X-Forwarded-For is
// appended to so a chain of proxies is preserved rather than overwritten, and
// X-Forwarded-Proto reflects the scheme the client actually used.
func (p *Proxy) setForwardHeaders(c *core.Context, req *fasthttp.Request) {
	ctx := c.RequestCtx
	ip := ctx.RemoteIP().String()

	if prior := string(ctx.Request.Header.Peek("X-Forwarded-For")); prior != "" {
		req.Header.Set("X-Forwarded-For", prior+", "+ip)
	} else {
		req.Header.Set("X-Forwarded-For", ip)
	}

	req.Header.Set("X-Forwarded-Host", string(ctx.Host()))
	req.Header.Set("X-Real-IP", ip)

	scheme := "http"
	if ctx.IsTLS() {
		scheme = "https"
	}
	if h := string(ctx.Request.Header.Peek("X-Forwarded-Proto")); h != "" {
		scheme = h
	}
	req.Header.Set("X-Forwarded-Proto", scheme)
}

// connectionTokens returns the header names listed by a Connection header.
func connectionTokens(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	names := make([]string, 0, len(parts))
	for _, name := range parts {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func stripRequestHopByHop(h *fasthttp.RequestHeader) {
	for _, name := range connectionTokens(string(h.Peek("Connection"))) {
		h.Del(name)
	}
	h.Del("Connection")
	h.Del("Proxy-Connection")
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}

func stripResponseHopByHop(h *fasthttp.ResponseHeader) {
	for _, name := range connectionTokens(string(h.Peek("Connection"))) {
		h.Del(name)
	}
	h.Del("Connection")
	h.Del("Proxy-Connection")
	for _, name := range hopByHopHeaders {
		h.Del(name)
	}
}
