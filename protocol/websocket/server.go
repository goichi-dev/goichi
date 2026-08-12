package websocket

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/fasthttp/websocket"
	"github.com/goichi-dev/goichi/core"
	"github.com/goichi-dev/goichi/middleware"
	"github.com/valyala/fasthttp"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type WebSocketServer struct {
	config      WebSocketConfig
	upgrader    websocket.FastHTTPUpgrader
	isRunning   bool
	mu          sync.RWMutex
	connections map[string]*WebSocketConn
	done        chan struct{}
	server      *fasthttp.Server

	outerMW func(next fasthttp.RequestHandler) fasthttp.RequestHandler

	OnConnect    func(conn *WebSocketConn)
	OnMessage    func(conn *WebSocketConn, messageType int, data []byte)
	OnDisconnect func(conn *WebSocketConn, err error)

	authSecret string
	authLookup string
}

type WebSocketConn struct {
	ID   string
	Conn *websocket.Conn
	Ctx  context.Context
}

func (ws *WebSocketServer) SetAuthSecret(secret string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.authSecret = secret
}

func (ws *WebSocketServer) SetAuthLookup(lookup string) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.authLookup = lookup
}

func (ws *WebSocketServer) SetListener(ln net.Listener) {
	// Not used directly as fasthttp handles it in Start
}

func NewWebSocketServer(config WebSocketConfig) *WebSocketServer {
	config.SetDefaults()
	ws := &WebSocketServer{
		config:      config,
		connections: make(map[string]*WebSocketConn),
		done:        make(chan struct{}),
	}
	ws.upgrader = websocket.FastHTTPUpgrader{
		ReadBufferSize:  config.ReadBufferSize,
		WriteBufferSize: config.WriteBufferSize,
		CheckOrigin:     ws.checkOrigin,
	}
	return ws
}

// checkOrigin defends against Cross-Site WebSocket Hijacking by validating the
// Origin header against the configured allowlist. With no allowlist configured
// it falls back to a same-origin policy.
func (ws *WebSocketServer) checkOrigin(ctx *fasthttp.RequestCtx) bool {
	origin := strings.TrimSpace(string(ctx.Request.Header.Peek("Origin")))
	if origin == "" {
		// Non-browser clients (and same-origin requests in some cases) omit Origin.
		return true
	}

	for _, o := range ws.config.AllowedOrigins {
		if o == "*" || strings.EqualFold(o, origin) {
			return true
		}
	}

	// Fall back to same-origin: compare the Origin host with the request Host.
	if len(ws.config.AllowedOrigins) == 0 {
		host := string(ctx.Host())
		if oh := originHost(origin); oh != "" && strings.EqualFold(oh, host) {
			return true
		}
	}

	log.Printf("[WebSocket] Rejected connection from disallowed origin: %s", origin)
	return false
}

func originHost(origin string) string {
	if i := strings.Index(origin, "://"); i >= 0 {
		return origin[i+3:]
	}
	return origin
}

// checkAuth validates the unified-auth JWT on the upgrade request when a secret
// is configured (or RequireAuth is set). Returns true when the request may proceed.
func (ws *WebSocketServer) checkAuth(ctx *fasthttp.RequestCtx) bool {
	ws.mu.RLock()
	secret := ws.authSecret
	lookup := ws.authLookup
	required := ws.config.RequireAuth
	ws.mu.RUnlock()

	if secret == "" && !required {
		return true
	}
	if secret == "" && required {
		log.Printf("[WebSocket] RequireAuth set but no auth secret configured")
		return false
	}

	c := &core.Context{RequestCtx: ctx}
	raw := middleware.ExtractToken(c, lookup)
	_, errMsg := middleware.ValidateJWT(raw, secret)
	return errMsg == ""
}

func (ws *WebSocketServer) GetInfo() string {
	return fmt.Sprintf("Path: %s (Port %d)", ws.config.Path, ws.config.Port)
}

func (ws *WebSocketServer) GetName() string {
	return "websocket"
}

func (ws *WebSocketServer) GetPaths() []string {
	return []string{ws.config.Path}
}

func (ws *WebSocketServer) SetFastHTTPOuterMiddleware(wrap func(next fasthttp.RequestHandler) fasthttp.RequestHandler) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.outerMW = wrap
}

func (ws *WebSocketServer) Handler() func(c *fasthttp.RequestCtx) {
	h := func(ctx *fasthttp.RequestCtx) {
		ws.handleRequest(ctx)
	}

	ws.mu.RLock()
	outer := ws.outerMW
	ws.mu.RUnlock()
	if outer != nil {
		return outer(h)
	}
	return h
}

func (ws *WebSocketServer) Start(ctx context.Context) error {
	ws.mu.Lock()
	if ws.isRunning {
		ws.mu.Unlock()
		return nil
	}
	ws.mu.Unlock()

	if !ws.config.Enabled {
		log.Printf("[WebSocket] Disabled, skipping start")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", ws.config.Address, ws.config.Port)

	handler := fasthttp.RequestHandler(ws.handleRequest)
	ws.mu.RLock()
	outer := ws.outerMW
	ws.mu.RUnlock()
	if outer != nil {
		handler = outer(handler)
	}

	ws.server = &fasthttp.Server{
		Handler: handler,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	if ws.config.TLSEnabled() {
		cert, err := tls.LoadX509KeyPair(ws.config.TLSCertFile, ws.config.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("websocket tls: %w", err)
		}
		ln = tls.NewListener(ln, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
	}

	ws.mu.Lock()
	ws.isRunning = true
	ws.mu.Unlock()

	go func() {
		if err := ws.server.Serve(ln); err != nil {
			log.Printf("[WebSocket] Server error: %v", err)
		}
	}()

	scheme := "ws"
	if ws.config.TLSEnabled() {
		scheme = "wss"
	}
	log.Printf("[WebSocket] Server started on %s (path: %s, scheme: %s)", addr, ws.config.Path, scheme)
	return nil
}

func (ws *WebSocketServer) handleRequest(ctx *fasthttp.RequestCtx) {
	// The path check stays permissive: the server may be mounted under a prefix
	// So we only check path if we are NOT running as a mounted handler?
	if ws.isRunning && string(ctx.Path()) != ws.config.Path {
		ctx.Error("Not Found", fasthttp.StatusNotFound)
		return
	}

	// Reject unauthenticated upgrades when unified auth is configured.
	if !ws.checkAuth(ctx) {
		ctx.Error("Unauthorized", fasthttp.StatusUnauthorized)
		return
	}

	// Enforce the maximum concurrent connection limit.
	ws.mu.RLock()
	atCapacity := len(ws.connections) >= ws.config.MaxConnections
	ws.mu.RUnlock()
	if atCapacity {
		ctx.Error("Too Many Connections", fasthttp.StatusServiceUnavailable)
		return
	}

	err := ws.upgrader.Upgrade(ctx, func(conn *websocket.Conn) {
		wsConn := &WebSocketConn{
			ID:   fmt.Sprintf("%s-%d", ctx.RemoteAddr(), time.Now().UnixNano()),
			Conn: conn,
			Ctx:  context.Background(),
		}

		ws.mu.Lock()
		ws.connections[wsConn.ID] = wsConn
		ws.mu.Unlock()

		if ws.OnConnect != nil {
			ws.OnConnect(wsConn)
		}

		connDone := make(chan struct{})

		defer func() {
			close(connDone)
			ws.mu.Lock()
			delete(ws.connections, wsConn.ID)
			ws.mu.Unlock()
			conn.Close()
			if ws.OnDisconnect != nil {
				ws.OnDisconnect(wsConn, nil)
			}
		}()

		// Set up ping-pong read deadlines
		pongWait := time.Duration(ws.config.PongWait) * time.Second
		pingInterval := time.Duration(ws.config.PingInterval) * time.Second

		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})

		// Ping sender goroutine
		go func() {
			ticker := time.NewTicker(pingInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					// Send Ping
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						return
					}
				case <-connDone:
					return
				case <-ws.done:
					return
				}
			}
		}()

		for {
			messageType, p, err := conn.ReadMessage()
			if err != nil {
				if ws.OnDisconnect != nil {
					ws.OnDisconnect(wsConn, err)
				}
				return
			}
			// Reset read deadline on message receipt
			conn.SetReadDeadline(time.Now().Add(pongWait))
			if ws.OnMessage != nil {
				ws.OnMessage(wsConn, messageType, p)
			}
		}
	})

	if err != nil {
		log.Printf("[WebSocket] Upgrade error: %v", err)
	}
}

func (ws *WebSocketServer) Stop(ctx context.Context) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if !ws.isRunning {
		return nil
	}

	if ws.server != nil {
		ws.server.Shutdown()
	}

	ws.isRunning = false
	close(ws.done)

	log.Printf("[WebSocket] Server stopped")
	return nil
}

func (ws *WebSocketServer) IsRunning() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return ws.isRunning
}

func (ws *WebSocketServer) Broadcast(data []byte) error {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	for _, conn := range ws.connections {
		conn.Conn.WriteMessage(websocket.TextMessage, data)
	}
	return nil
}

func (ws *WebSocketServer) SendToClient(clientID string, messageType int, data []byte) error {
	ws.mu.RLock()
	conn, exists := ws.connections[clientID]
	ws.mu.RUnlock()

	if !exists {
		return fmt.Errorf("client %s not found", clientID)
	}

	return conn.Conn.WriteMessage(messageType, data)
}

func (ws *WebSocketServer) GetConnections() map[string]*WebSocketConn {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	conns := make(map[string]*WebSocketConn)
	for id, conn := range ws.connections {
		conns[id] = conn
	}
	return conns
}
