package mcp

import "github.com/goichi-dev/goichi/protocol"

// MCPConfig configures the Model Context Protocol server.
//
// MCP speaks JSON-RPC over a raw TCP connection, not HTTP, so the server always
// opens its own listener and is never mounted on the shared HTTP router — there
// is no URL path to configure.
type MCPConfig struct {
	protocol.ProtocolConfig

	// MaxToolCount caps how many tools may be registered. Default 100.
	MaxToolCount int

	// MaxResourceCount caps how many resources may be registered. Default 100.
	MaxResourceCount int

	// Timeout is the per-request deadline in seconds. Default 30.
	Timeout int

	// MaxRequestBytes caps the size of a single JSON-RPC request to prevent
	// memory-exhaustion DoS. Default 1MB.
	MaxRequestBytes int64

	// RequireAuth rejects all requests until the client authenticates. Implied
	// automatically when a unified auth secret is configured.
	RequireAuth bool
}

func (c *MCPConfig) SetDefaults() {
	if c.Address == "" {
		c.Address = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8083
	}
	if c.MaxToolCount == 0 {
		c.MaxToolCount = 100
	}
	if c.MaxResourceCount == 0 {
		c.MaxResourceCount = 100
	}
	if c.Timeout == 0 {
		c.Timeout = 30
	}
	if c.MaxRequestBytes <= 0 {
		c.MaxRequestBytes = 1 << 20 // 1MB
	}
}
