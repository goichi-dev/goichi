package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/goichi-dev/goichi/protocol"
)

// TCPConn describes one accepted connection. It is passed to the hooks so they
// can decide, log, or rewrite without depending on the proxy internals.
type TCPConn struct {
	// Client is the accepted downstream connection.
	Client net.Conn
	// Target is the upstream chosen for this connection, nil before the
	// balancer has run (in OnConnect).
	Target *Target
	// Locals carries values between hooks for the lifetime of the connection.
	Locals map[string]any
}

// Set stores a value for the lifetime of the connection.
func (c *TCPConn) Set(key string, val any) {
	if c.Locals == nil {
		c.Locals = make(map[string]any)
	}
	c.Locals[key] = val
}

// Get returns a value stored by Set, or nil.
func (c *TCPConn) Get(key string) any {
	if c.Locals == nil {
		return nil
	}
	return c.Locals[key]
}

// TCPConfig configures a raw TCP proxy. It fronts any byte-stream service:
// Postgres, MySQL, Redis, an SMTP relay or a plain socket server.
type TCPConfig struct {
	protocol.ProtocolConfig

	// Name distinguishes several TCP proxies in one app, and is the protocol
	// name in the registry. Default "tcp-proxy".
	Name string
	// Target is a single upstream "host:port". Use Targets to balance.
	Target string
	// Targets is the upstream pool. When set, Target is ignored.
	Targets []*Target
	// Balancer selects a target per connection. Default RoundRobin.
	Balancer Algorithm
	// DialTimeout is the deadline for connecting upstream. Default 10s.
	DialTimeout time.Duration
	// IdleTimeout closes a connection after this long with no bytes in either
	// direction. Zero disables the idle timeout.
	IdleTimeout time.Duration
	// MaxConnections caps concurrent connections. Zero means unlimited.
	MaxConnections int64
	// HealthCheck configures background probing. Probes open a TCP connection
	// to each target; HealthCheck.Path is ignored.
	HealthCheck HealthCheck
	// UpstreamTLS wraps the connection to the upstream in TLS, for a database
	// or service that requires it.
	UpstreamTLS *tls.Config

	// OnConnect runs after accept and before a target is chosen. Returning an
	// error closes the connection without dialing upstream, which is where IP
	// allow-lists and connection policy belong.
	OnConnect func(c *TCPConn) error
	// OnClientData sees each chunk read from the client before it is written
	// upstream, and returns the bytes to forward. Returning an error closes
	// the connection. This is the inspection point for a wire protocol: read
	// the Postgres startup packet, gate a Redis command, count queries.
	OnClientData func(c *TCPConn, b []byte) ([]byte, error)
	// OnUpstreamData is the same for bytes flowing back to the client.
	OnUpstreamData func(c *TCPConn, b []byte) ([]byte, error)
	// OnClose runs once the connection is finished, with the bytes copied in
	// each direction.
	OnClose func(c *TCPConn, sent, received int64)
	// ErrorHandler receives connection-level errors. Default logs them.
	ErrorHandler func(c *TCPConn, err error)
}

// SetDefaults fills in the name, timeouts and normalized targets.
func (c *TCPConfig) SetDefaults() {
	if c.Name == "" {
		c.Name = "tcp-proxy"
	}
	if c.Address == "" {
		c.Address = "0.0.0.0"
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = 10 * time.Second
	}
	for _, t := range c.Targets {
		t.URL = hostPort(t.URL)
	}
	if c.Target != "" {
		c.Target = hostPort(c.Target)
	}
}

// TCPProxy is a layer-4 reverse proxy exposed as a goichi protocol server, so
// it starts, stops and reports alongside the HTTP, gRPC and MQTT servers.
type TCPProxy struct {
	config  TCPConfig
	bal     *Balancer
	checker *checker

	listener net.Listener
	injected bool

	active    atomic.Int64
	total     atomic.Uint64
	isRunning bool
	mu        sync.RWMutex

	cancel context.CancelFunc
	conns  sync.WaitGroup
}

// NewTCPProxy returns a TCP proxy for config. Register it with
// App.RegisterProtocol to have the app manage its lifecycle.
func NewTCPProxy(config TCPConfig) *TCPProxy {
	config.SetDefaults()

	targets := config.Targets
	if len(targets) == 0 && config.Target != "" {
		targets = []*Target{{URL: config.Target}}
	}

	p := &TCPProxy{
		config: config,
		bal:    NewBalancer(targets, config.Balancer),
	}
	p.checker = newChecker(config.HealthCheck, p.bal, tcpProbe(config.HealthCheck, config.UpstreamTLS))
	return p
}

// Config returns a pointer to the live configuration, so hooks can be attached
// after construction. Change it before Start; the proxy reads the hooks on
// every connection.
func (p *TCPProxy) Config() *TCPConfig { return &p.config }

// GetName returns the protocol name used in the registry.
func (p *TCPProxy) GetName() string { return p.config.Name }

// GetInfo returns a one-line description for startup logging and discovery.
func (p *TCPProxy) GetInfo() string {
	return fmt.Sprintf("TCP proxy on %s:%d -> %d target(s)",
		p.config.Address, p.config.Port, len(p.bal.Targets()))
}

// GetPaths returns nil: a TCP proxy owns a port, not HTTP paths.
func (p *TCPProxy) GetPaths() []string { return nil }

// SetListener injects a listener, letting the proxy share the main port under
// cmux port multiplexing instead of binding its own.
func (p *TCPProxy) SetListener(ln net.Listener) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listener = ln
	p.injected = true
}

// IsRunning reports whether the proxy is accepting connections.
func (p *TCPProxy) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.isRunning
}

// Targets returns the upstream pool, so callers can report health.
func (p *TCPProxy) Targets() []*Target { return p.bal.Targets() }

// Stats returns the number of connections currently open and the number
// accepted since start.
func (p *TCPProxy) Stats() (active int64, total uint64) {
	return p.active.Load(), p.total.Load()
}

// Start binds the port (unless a listener was injected) and serves until Stop.
func (p *TCPProxy) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.isRunning {
		p.mu.Unlock()
		return nil
	}
	if !p.config.Enabled {
		p.mu.Unlock()
		log.Printf("[%s] Disabled, skipping start", p.config.Name)
		return nil
	}
	if len(p.bal.Targets()) == 0 {
		p.mu.Unlock()
		return fmt.Errorf("%s: no target configured", p.config.Name)
	}

	if p.listener == nil {
		addr := fmt.Sprintf("%s:%d", p.config.Address, p.config.Port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			p.mu.Unlock()
			return fmt.Errorf("%s: listen on %s: %w", p.config.Name, addr, err)
		}
		if p.config.TLSEnabled() {
			cert, err := tls.LoadX509KeyPair(p.config.TLSCertFile, p.config.TLSKeyFile)
			if err != nil {
				_ = ln.Close()
				p.mu.Unlock()
				return fmt.Errorf("%s: load TLS keypair: %w", p.config.Name, err)
			}
			ln = tls.NewListener(ln, &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			})
		}
		p.listener = ln
	}

	serveCtx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.isRunning = true
	ln := p.listener
	p.mu.Unlock()

	p.checker.start()

	go p.acceptLoop(serveCtx, ln)
	log.Printf("[%s] %s", p.config.Name, p.GetInfo())
	return nil
}

// Stop closes the listener and waits for in-flight connections to finish, or
// for ctx to be done.
func (p *TCPProxy) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.isRunning {
		p.mu.Unlock()
		return nil
	}
	p.isRunning = false
	cancel := p.cancel
	ln := p.listener
	injected := p.injected
	p.listener = nil
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	p.checker.stop()

	// An injected listener belongs to cmux, which closes it itself.
	if ln != nil && !injected {
		_ = ln.Close()
	}

	done := make(chan struct{})
	go func() {
		p.conns.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
	log.Printf("[%s] Stopped", p.config.Name)
	return nil
}

func (p *TCPProxy) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return
		}

		if max := p.config.MaxConnections; max > 0 && p.active.Load() >= max {
			p.reportErr(&TCPConn{Client: conn}, fmt.Errorf("connection limit of %d reached", max))
			_ = conn.Close()
			continue
		}

		p.conns.Add(1)
		p.active.Add(1)
		p.total.Add(1)
		go func() {
			defer p.conns.Done()
			defer p.active.Add(-1)
			p.handle(ctx, conn)
		}()
	}
}

func (p *TCPProxy) handle(ctx context.Context, client net.Conn) {
	tc := &TCPConn{Client: client}
	defer client.Close()

	if p.config.OnConnect != nil {
		if err := p.config.OnConnect(tc); err != nil {
			p.reportErr(tc, err)
			return
		}
	}

	host, _, err := net.SplitHostPort(client.RemoteAddr().String())
	if err != nil {
		host = client.RemoteAddr().String()
	}
	target := p.bal.Pick(host)
	if target == nil {
		p.reportErr(tc, ErrNoTarget)
		return
	}
	tc.Target = target

	upstream, err := dialUpstream(target.URL, p.config.UpstreamTLS != nil, p.config.UpstreamTLS, p.config.DialTimeout)
	if err != nil {
		target.healthy.Store(false)
		p.reportErr(tc, fmt.Errorf("dial %s: %w", target.URL, err))
		return
	}
	defer upstream.Close()

	target.active.Add(1)
	defer target.active.Add(-1)

	// Close both sides as soon as the app is shutting down, so a long-lived
	// database session cannot hold Stop open past its context.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
			_ = upstream.Close()
		case <-stop:
		}
	}()

	sent, received := p.relay(tc, client, upstream)

	if p.config.OnClose != nil {
		p.config.OnClose(tc, sent, received)
	}
}

// relay copies bytes in both directions, applying the data hooks and the idle
// timeout, and returns the number of bytes sent upstream and received back.
func (p *TCPProxy) relay(tc *TCPConn, client, upstream net.Conn) (sent, received int64) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Each goroutine owns one counter and wg.Wait happens-before the read, so
	// the plain assignments below are race-free.
	go func() {
		defer wg.Done()
		n, err := p.copy(tc, upstream, client, p.config.OnClientData)
		sent = n
		if err != nil {
			p.reportErr(tc, err)
		}
		closeWrite(upstream)
	}()

	go func() {
		defer wg.Done()
		n, err := p.copy(tc, client, upstream, p.config.OnUpstreamData)
		received = n
		if err != nil {
			p.reportErr(tc, err)
		}
		closeWrite(client)
	}()

	wg.Wait()
	return sent, received
}

// copy moves bytes from src to dst, passing each chunk through hook when set.
// A hook returning no bytes drops the chunk without closing the connection.
func (p *TCPProxy) copy(tc *TCPConn, dst, src net.Conn, hook func(*TCPConn, []byte) ([]byte, error)) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64

	for {
		if p.config.IdleTimeout > 0 {
			_ = src.SetReadDeadline(time.Now().Add(p.config.IdleTimeout))
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if hook != nil {
				out, err := hook(tc, chunk)
				if err != nil {
					return total, err
				}
				chunk = out
			}
			if len(chunk) > 0 {
				if p.config.IdleTimeout > 0 {
					_ = dst.SetWriteDeadline(time.Now().Add(p.config.IdleTimeout))
				}
				written, writeErr := dst.Write(chunk)
				total += int64(written)
				if writeErr != nil {
					return total, writeErr
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

func (p *TCPProxy) reportErr(tc *TCPConn, err error) {
	if err == nil || isClosedConn(err) {
		return
	}
	if p.config.ErrorHandler != nil {
		p.config.ErrorHandler(tc, err)
		return
	}
	log.Printf("[%s] %v", p.config.Name, err)
}

// isClosedConn reports whether err is the ordinary result of either side
// hanging up, which happens on every normal close and on shutdown. Reporting
// it as an error would make routine teardown look like a fault.
func isClosedConn(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED)
}
