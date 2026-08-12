package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/goichi-dev/goichi/middleware"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
)

// ConnectServer implements Code-first RPC using ConnectRPC
type ConnectServer struct {
	config    GRPCConfig
	isRunning bool
	mu        sync.RWMutex
	listener  net.Listener
	mux       *http.ServeMux
	server    *http.Server

	authSecret string
	authLookup string
}

func (cs *ConnectServer) SetAuthSecret(secret string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.authSecret = secret
}

func (cs *ConnectServer) SetAuthLookup(lookup string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.authLookup = lookup
}

// authMiddleware enforces a valid JWT (from the Authorization header) when a
// unified auth secret is configured.
func (cs *ConnectServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.RLock()
		secret := cs.authSecret
		cs.mu.RUnlock()

		if secret != "" {
			raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if _, errMsg := middleware.ValidateJWT(raw, secret); errMsg != "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func NewConnectServer(config GRPCConfig) *ConnectServer {
	config.SetDefaults()
	return &ConnectServer{
		config: config,
		mux:    http.NewServeMux(),
	}
}

// Register maps a service implementation to ConnectRPC
// Note: Code-first in ConnectRPC usually requires a generated path,
// but here we are setting up the framework to host it.
func (cs *ConnectServer) Register(path string, handler http.Handler) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.mux.Handle(path, handler)
}

func (cs *ConnectServer) HTTPHandler() http.Handler {
	return cs.authMiddleware(cs.mux)
}

func (cs *ConnectServer) GetName() string {
	return "connect"
}

func (cs *ConnectServer) GetPaths() []string {
	return nil
}

func (cs *ConnectServer) Start(ctx context.Context) error {
	cs.mu.Lock()
	if cs.isRunning {
		cs.mu.Unlock()
		return nil
	}

	addr := fmt.Sprintf("%s:%d", cs.config.Address, cs.config.Port)
	cs.server = &http.Server{
		Addr:    addr,
		Handler: cs.authMiddleware(cs.mux),
	}

	ln := cs.listener
	if ln == nil {
		var err error
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			cs.mu.Unlock()
			return err
		}
	}

	if cs.config.TLSEnabled() {
		cert, err := tls.LoadX509KeyPair(cs.config.TLSCertFile, cs.config.TLSKeyFile)
		if err != nil {
			cs.mu.Unlock()
			return fmt.Errorf("connect tls: %w", err)
		}
		ln = tls.NewListener(ln, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		})
	}

	cs.listener = ln
	cs.isRunning = true
	cs.mu.Unlock()

	go func() {
		if err := cs.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[Connect] Server error: %v", err)
		}
	}()

	log.Printf("[Connect] Server started on %s", addr)
	return nil
}

func (cs *ConnectServer) Stop(ctx context.Context) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.isRunning {
		return nil
	}

	cs.isRunning = false
	return cs.server.Shutdown(ctx)
}

func (cs *ConnectServer) IsRunning() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.isRunning
}

func (cs *ConnectServer) GetInfo() string {
	return fmt.Sprintf("ConnectRPC Service (Port %d)", cs.config.Port)
}

func (cs *ConnectServer) SetListener(ln net.Listener) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.listener = ln
}
