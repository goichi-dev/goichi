package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

// HealthCheck configures active probing of the target pool. A target that fails
// FailThreshold consecutive probes is taken out of rotation and put back after
// SuccessThreshold consecutive successes.
type HealthCheck struct {
	// Enable turns on background probing. Without it every target stays in
	// rotation and failures are only noticed when a request fails.
	Enable bool
	// Interval is the time between probes. Default 10s.
	Interval time.Duration
	// Timeout is the per-probe deadline. Default 2s.
	Timeout time.Duration
	// Path is the HTTP path to probe, e.g. "/health". Ignored by the TCP
	// proxy, which probes by opening a connection. Default "/".
	Path string
	// FailThreshold is the number of consecutive failures that marks a target
	// unhealthy. Default 3.
	FailThreshold int64
	// SuccessThreshold is the number of consecutive successes that restores a
	// target. Default 1.
	SuccessThreshold int64
}

func (h *HealthCheck) setDefaults() {
	if h.Interval <= 0 {
		h.Interval = 10 * time.Second
	}
	if h.Timeout <= 0 {
		h.Timeout = 2 * time.Second
	}
	if h.Path == "" {
		h.Path = "/"
	}
	if h.FailThreshold <= 0 {
		h.FailThreshold = 3
	}
	if h.SuccessThreshold <= 0 {
		h.SuccessThreshold = 1
	}
}

// probeFunc reports whether one target is currently reachable.
type probeFunc func(t *Target) bool

// checker runs the probe loop for one balancer.
type checker struct {
	cfg     HealthCheck
	bal     *Balancer
	probe   probeFunc
	cancel  context.CancelFunc
	stopped sync.WaitGroup
}

func newChecker(cfg HealthCheck, bal *Balancer, probe probeFunc) *checker {
	cfg.setDefaults()
	return &checker{cfg: cfg, bal: bal, probe: probe}
}

// start begins probing in the background. It is a no-op when health checking
// is disabled or already running.
func (c *checker) start() {
	if !c.cfg.Enable || c.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.stopped.Add(1)
	go func() {
		defer c.stopped.Done()
		ticker := time.NewTicker(c.cfg.Interval)
		defer ticker.Stop()
		c.runOnce()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.runOnce()
			}
		}
	}()
}

// stop ends the probe loop and waits for it to exit.
func (c *checker) stop() {
	if c.cancel == nil {
		return
	}
	c.cancel()
	c.stopped.Wait()
	c.cancel = nil
}

// runOnce probes every target in parallel and applies the thresholds. fails
// counts consecutive failures while unhealthy targets count consecutive
// successes in the same field, so one counter drives both transitions.
func (c *checker) runOnce() {
	var wg sync.WaitGroup
	for _, t := range c.bal.Targets() {
		wg.Add(1)
		go func(t *Target) {
			defer wg.Done()
			ok := c.probe(t)
			switch {
			case ok && t.healthy.Load():
				t.fails.Store(0)
			case ok:
				if t.fails.Add(1) >= c.cfg.SuccessThreshold {
					t.fails.Store(0)
					t.healthy.Store(true)
				}
			case !ok && !t.healthy.Load():
				t.fails.Store(0)
			default:
				if t.fails.Add(1) >= c.cfg.FailThreshold {
					t.fails.Store(0)
					t.healthy.Store(false)
				}
			}
		}(t)
	}
	wg.Wait()
}

// httpProbe returns a probe that requests cfg.Path on the target and treats any
// status below 400 as healthy.
func httpProbe(cfg HealthCheck, client *fasthttp.Client) probeFunc {
	cfg.setDefaults()
	path := cfg.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return func(t *Target) bool {
		req := fasthttp.AcquireRequest()
		res := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(res)

		req.SetRequestURI(strings.TrimSuffix(t.URL, "/") + path)
		req.Header.SetMethod(fasthttp.MethodGet)
		if err := client.DoTimeout(req, res, cfg.Timeout); err != nil {
			return false
		}
		return res.StatusCode() < 400
	}
}

// tcpProbe returns a probe that reports a target healthy when a TCP (or TLS)
// connection to it can be established within the timeout.
func tcpProbe(cfg HealthCheck, tlsCfg *tls.Config) probeFunc {
	cfg.setDefaults()
	return func(t *Target) bool {
		addr := hostPort(t.URL)
		var conn net.Conn
		var err error
		if tlsCfg != nil {
			d := &net.Dialer{Timeout: cfg.Timeout}
			conn, err = tls.DialWithDialer(d, "tcp", addr, tlsCfg)
		} else {
			conn, err = net.DialTimeout("tcp", addr, cfg.Timeout)
		}
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
}

// hostPort reduces a target address to "host:port", accepting both a bare
// address and a URL so the two proxies can share one Target type.
func hostPort(addr string) string {
	if i := strings.Index(addr, "://"); i >= 0 {
		if u, err := url.Parse(addr); err == nil && u.Host != "" {
			return u.Host
		}
		return addr[i+3:]
	}
	return addr
}
