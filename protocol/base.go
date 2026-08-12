package protocol

import (
	"context"
	"net"
)

type ProtocolHandler interface {
	GetName() string

	Start(ctx context.Context) error

	Stop(ctx context.Context) error

	IsRunning() bool

	// GetInfo returns discovery information for the protocol (e.g., endpoint, port)
	GetInfo() string

	// GetPaths returns the HTTP paths handled by this protocol (if applicable)
	GetPaths() []string

	// SetListener injects a listener for the protocol
	SetListener(ln net.Listener)
}

type ProtocolConfig struct {
	Enabled bool
	Port    int
	Address string

	// TLSCertFile and TLSKeyFile, when both set, enable TLS for the protocol's
	// own listener (wss / grpcs / mqtts / secure MCP). Leave empty for plaintext
	// (e.g. when TLS is terminated by an upstream reverse proxy).
	TLSCertFile string
	TLSKeyFile  string
}

// TLSEnabled reports whether both a certificate and key were configured.
func (c ProtocolConfig) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}

// AuthAware is implemented by protocols that support unified authentication
type AuthAware interface {
	ProtocolHandler
	SetAuthSecret(secret string)
	SetAuthLookup(lookup string)
}

type ProtocolRegistry struct {
	handlers map[string]ProtocolHandler
}

func NewProtocolRegistry() *ProtocolRegistry {
	return &ProtocolRegistry{
		handlers: make(map[string]ProtocolHandler),
	}
}

func (r *ProtocolRegistry) Register(handler ProtocolHandler) {
	r.handlers[handler.GetName()] = handler
}

func (r *ProtocolRegistry) Get(name string) ProtocolHandler {
	return r.handlers[name]
}

func (r *ProtocolRegistry) GetAll() map[string]ProtocolHandler {
	return r.handlers
}

func (r *ProtocolRegistry) SetUnifiedAuth(secret, lookup string) {
	for _, handler := range r.handlers {
		if aa, ok := handler.(AuthAware); ok {
			aa.SetAuthSecret(secret)
			aa.SetAuthLookup(lookup)
		}
	}
}

func (r *ProtocolRegistry) StartAll(ctx context.Context) error {
	for _, handler := range r.handlers {
		if err := handler.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProtocolRegistry) StopAll(ctx context.Context) error {
	for _, handler := range r.handlers {
		if handler.IsRunning() {
			if err := handler.Stop(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
