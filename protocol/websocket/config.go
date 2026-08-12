package websocket

import "github.com/goichi-dev/goichi/protocol"

type WebSocketConfig struct {
	protocol.ProtocolConfig
	Path             string // default "/ws"
	MaxConnections   int    // default 1000
	ReadBufferSize   int    // default 1024
	WriteBufferSize  int    // default 1024
	HandshakeTimeout int    // seconds, default 5
	PingInterval     int    // seconds, default 54
	PongWait         int    // seconds, default 60

	// AllowedOrigins is the allowlist of Origin headers permitted to open a
	// WebSocket connection (defends against Cross-Site WebSocket Hijacking).
	// Use ["*"] to explicitly allow any origin. When empty, only same-origin
	// requests (Origin matches Host) and requests with no Origin are allowed.
	AllowedOrigins []string

	// RequireAuth, when true, rejects the upgrade unless a valid JWT is present
	// (per the unified auth lookup). It is implied automatically when a unified
	// auth secret is configured.
	RequireAuth bool
}

func (c *WebSocketConfig) SetDefaults() {
	if c.Address == "" {
		c.Address = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	if c.Path == "" {
		c.Path = "/ws"
	}
	if c.MaxConnections == 0 {
		c.MaxConnections = 1000
	}
	if c.ReadBufferSize == 0 {
		c.ReadBufferSize = 1024
	}
	if c.WriteBufferSize == 0 {
		c.WriteBufferSize = 1024
	}
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = 5
	}
	if c.PongWait == 0 {
		c.PongWait = 60
	}
	if c.PingInterval == 0 {
		c.PingInterval = (c.PongWait * 9) / 10
	}
}
