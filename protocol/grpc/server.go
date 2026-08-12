package grpc

import (
	"context"
	"fmt"
	"github.com/goichi-dev/goichi/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type GRPCServer struct {
	config    GRPCConfig
	server    *grpc.Server
	isRunning bool
	mu        sync.RWMutex
	listener  net.Listener

	RegisterFunc func(*grpc.Server)

	unarySrc  func() []grpc.UnaryServerInterceptor
	streamSrc func() []grpc.StreamServerInterceptor

	authSecret string
	authLookup string
}

func (gs *GRPCServer) SetListener(ln net.Listener) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.listener = ln
}

func (gs *GRPCServer) SetAuthSecret(secret string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.authSecret = secret
}

func (gs *GRPCServer) SetAuthLookup(lookup string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.authLookup = lookup
}

func NewGRPCServer(config GRPCConfig) *GRPCServer {
	config.SetDefaults()
	return &GRPCServer{
		config: config,
	}
}

func (gs *GRPCServer) SetUnaryInterceptorSource(src func() []grpc.UnaryServerInterceptor) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.unarySrc = src
}

func (gs *GRPCServer) SetStreamInterceptorSource(src func() []grpc.StreamServerInterceptor) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.streamSrc = src
}

func (gs *GRPCServer) GetInfo() string {
	return fmt.Sprintf("gRPC Service (Port %d)", gs.config.Port)
}

func (gs *GRPCServer) GetName() string {
	return "grpc"
}

func (gs *GRPCServer) GetPaths() []string {
	return nil
}

func (gs *GRPCServer) Start(ctx context.Context) error {
	gs.mu.Lock()
	if gs.isRunning {
		gs.mu.Unlock()
		return nil
	}
	gs.mu.Unlock()

	if !gs.config.Enabled {
		log.Printf("[gRPC] Disabled, skipping start")
		return nil
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(gs.config.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(gs.config.MaxSendMsgSize),
		grpc.MaxConcurrentStreams(gs.config.MaxConcurrentStreams),
	}

	if gs.config.KeepaliveTime > 0 {
		opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
			Time: time.Duration(gs.config.KeepaliveTime) * time.Second,
		}))
	}

	if gs.config.TLSEnabled() {
		creds, err := credentials.NewServerTLSFromFile(gs.config.TLSCertFile, gs.config.TLSKeyFile)
		if err != nil {
			return fmt.Errorf("grpc tls: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	var interceptors []grpc.UnaryServerInterceptor
	var streamInterceptors []grpc.StreamServerInterceptor

	gs.mu.RLock()
	secret := gs.authSecret
	src := gs.unarySrc
	streamSrc := gs.streamSrc
	gs.mu.RUnlock()

	// Add Unified Auth Interceptor if secret is provided
	if secret != "" {
		interceptors = append(interceptors, gs.unifiedAuthInterceptor(secret))
		streamInterceptors = append(streamInterceptors, gs.unifiedAuthStreamInterceptor(secret))
	}

	if src != nil {
		if ints := src(); len(ints) > 0 {
			interceptors = append(interceptors, ints...)
		}
	}

	if streamSrc != nil {
		if ints := streamSrc(); len(ints) > 0 {
			streamInterceptors = append(streamInterceptors, ints...)
		}
	}

	if len(interceptors) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(interceptors...))
	}

	if len(streamInterceptors) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(streamInterceptors...))
	}

	gs.mu.Lock()
	gs.server = grpc.NewServer(opts...)
	gs.mu.Unlock()

	if gs.RegisterFunc != nil {
		gs.RegisterFunc(gs.server)
	}

	var listener net.Listener
	var err error
	var addr string

	gs.mu.RLock()
	listener = gs.listener
	gs.mu.RUnlock()

	if listener == nil {
		addr = fmt.Sprintf("%s:%d", gs.config.Address, gs.config.Port)
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	} else {
		addr = "injected listener"
	}

	gs.mu.Lock()
	gs.isRunning = true
	gs.mu.Unlock()

	go func() {
		if err := gs.server.Serve(listener); err != nil {
			log.Printf("[gRPC] Server error: %v", err)
		}
	}()

	log.Printf("[gRPC] Server started on %s", addr)
	return nil
}

func (gs *GRPCServer) Stop(ctx context.Context) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if !gs.isRunning {
		return nil
	}

	gs.server.GracefulStop()
	gs.isRunning = false

	log.Printf("[gRPC] Server stopped")
	return nil
}

func (gs *GRPCServer) IsRunning() bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.isRunning
}

func (gs *GRPCServer) GetServer() *grpc.Server {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.server
}

type jwtClaimsCtxKey struct{}

func (gs *GRPCServer) unifiedAuthInterceptor(secret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		vals := md.Get("authorization")
		if len(vals) == 0 {
			vals = md.Get("Authorization")
		}

		var raw string
		if len(vals) > 0 {
			raw = strings.TrimPrefix(strings.TrimSpace(vals[0]), "Bearer ")
		}

		if raw == "" {
			return nil, status.Error(codes.Unauthenticated, "missing or empty token")
		}

		claims, errMsg := middleware.DecodeJWT(raw, secret)
		if errMsg != "" {
			return nil, status.Error(codes.Unauthenticated, errMsg)
		}

		// Inject claims into context
		ctx = context.WithValue(ctx, jwtClaimsCtxKey{}, claims)
		return handler(ctx, req)
	}
}

// GetClaims retrieves JWT claims from gRPC context if present
func GetClaims(ctx context.Context) middleware.JWTClaims {
	v := ctx.Value(jwtClaimsCtxKey{})
	if v == nil {
		return nil
	}
	c, _ := v.(middleware.JWTClaims)
	return c
}

func (gs *GRPCServer) unifiedAuthStreamInterceptor(secret string) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return status.Error(codes.Unauthenticated, "missing metadata")
		}

		vals := md.Get("authorization")
		if len(vals) == 0 {
			vals = md.Get("Authorization")
		}

		var raw string
		if len(vals) > 0 {
			raw = strings.TrimPrefix(strings.TrimSpace(vals[0]), "Bearer ")
		}

		if raw == "" {
			return status.Error(codes.Unauthenticated, "missing or empty token")
		}

		claims, errMsg := middleware.DecodeJWT(raw, secret)
		if errMsg != "" {
			return status.Error(codes.Unauthenticated, errMsg)
		}

		// Inject claims into context of wrapped stream
		wrapped := newWrappedServerStream(ss)
		wrapped.wrappedContext = context.WithValue(ctx, jwtClaimsCtxKey{}, claims)
		return handler(srv, wrapped)
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	wrappedContext context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.wrappedContext
}

func newWrappedServerStream(ss grpc.ServerStream) *wrappedServerStream {
	return &wrappedServerStream{
		ServerStream:   ss,
		wrappedContext: ss.Context(),
	}
}
