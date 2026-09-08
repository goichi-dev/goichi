package proxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/goichi-dev/goichi/core"
	"github.com/valyala/fasthttp"
)

// isUpgrade reports whether the request asks to switch protocols, which covers
// WebSocket and any other Upgrade-based protocol.
func isUpgrade(req *fasthttp.Request) bool {
	if len(req.Header.Peek("Upgrade")) == 0 {
		return false
	}
	for _, tok := range connectionTokens(string(req.Header.Peek("Connection"))) {
		if strings.EqualFold(tok, "Upgrade") {
			return true
		}
	}
	return false
}

// doUpgrade forwards an Upgrade request by dialing the upstream, replaying the
// handshake, and then piping raw bytes in both directions for the lifetime of
// the connection.
func (p *Proxy) doUpgrade(c *core.Context) error {
	ctx := c.RequestCtx

	target := p.bal.Pick(ctx.RemoteIP().String())
	if target == nil {
		return p.fail(c, ErrNoTarget)
	}

	// Build the handshake to replay upstream. Hop-by-hop headers are dropped
	// except Connection/Upgrade, which are exactly what this request is for.
	req := fasthttp.AcquireRequest()
	ctx.Request.CopyTo(req)
	p.setForwardHeaders(c, req)
	req.SetRequestURI(p.rewritePath(string(ctx.Path())))
	if q := ctx.URI().QueryString(); len(q) > 0 {
		req.SetRequestURI(string(req.RequestURI()) + "?" + string(q))
	}
	if p.cfg.Host != "" {
		req.SetHost(p.cfg.Host)
	} else {
		req.SetHost(string(ctx.Host()))
	}
	if p.cfg.ModifyRequest != nil {
		p.cfg.ModifyRequest(c, req)
	}

	handshake := req.Header.Header()
	fasthttp.ReleaseRequest(req)

	secure := strings.HasPrefix(target.URL, "https://")
	addr := hostPort(target.URL)

	target.active.Add(1)
	upstream, err := dialUpstream(addr, secure, p.cfg.TLSConfig, p.cfg.Timeout)
	if err != nil {
		target.active.Add(-1)
		return p.fail(c, err)
	}

	// Hijack runs after the handler returns, once fasthttp hands over the raw
	// connection. Everything below happens on that connection, not on ctx.
	ctx.HijackSetNoResponse(true)
	ctx.Hijack(func(client net.Conn) {
		defer target.active.Add(-1)
		defer upstream.Close()

		if _, err := upstream.Write(handshake); err != nil {
			return
		}

		// Read the upstream handshake response and relay it verbatim, so the
		// client sees the real 101 (or the real rejection).
		br := bufio.NewReader(upstream)
		var res fasthttp.Response
		if err := res.Header.Read(br); err != nil {
			return
		}
		if _, err := client.Write(res.Header.Header()); err != nil {
			return
		}

		// Bytes may already sit in the reader's buffer after the header.
		if n := br.Buffered(); n > 0 {
			if buf, err := br.Peek(n); err == nil {
				if _, err := client.Write(buf); err != nil {
					return
				}
			}
		}

		pipe(client, upstream)
	})
	return nil
}

// dialUpstream opens a plain or TLS connection to addr.
func dialUpstream(addr string, secure bool, tlsCfg *tls.Config, timeout time.Duration) (net.Conn, error) {
	d := &net.Dialer{Timeout: timeout}
	if !secure {
		return d.Dial("tcp", addr)
	}
	cfg := tlsCfg
	if cfg == nil {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		cfg = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
	return tls.DialWithDialer(d, "tcp", addr, cfg)
}

// pipe copies bytes in both directions until either side closes, then unblocks
// the other by closing it too.
func pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		closeWrite(b)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		closeWrite(a)
	}()
	wg.Wait()
}

// closeWrite half-closes a connection when the transport supports it, so the
// peer sees EOF while still being able to flush its own remaining bytes.
func closeWrite(c net.Conn) {
	type closeWriter interface{ CloseWrite() error }
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = c.Close()
}
