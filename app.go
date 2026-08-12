package goichi

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/goichi-dev/goichi/core"
	"github.com/goichi-dev/goichi/middleware"
	"github.com/goichi-dev/goichi/openapi"
	"github.com/goichi-dev/goichi/protocol"
	"github.com/soheilhy/cmux"
	"github.com/valyala/fasthttp"
	"google.golang.org/grpc"
)

// ServerConfig controls the HTTP listener and process model.
type ServerConfig struct {
	AppName                string
	Multiprocess           bool
	NumChildren            int
	ShowStartup            *bool
	StartupBanner          string
	EnablePortMultiplexing bool
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	IdleTimeout            time.Duration
	MaxRequestBodySize     int

	// TLSCertFile and TLSKeyFile, when both set, serve the main HTTP listener
	// over TLS (https). Leave empty when TLS is terminated upstream.
	TLSCertFile string
	TLSKeyFile  string
}

// TLSEnabled reports whether both a certificate and key were configured for the
// main HTTP listener.
func (c *ServerConfig) TLSEnabled() bool {
	return c.TLSCertFile != "" && c.TLSKeyFile != ""
}

// SetDefaults fills in the timeouts, body limit and application name that were
// left at their zero value.
func (c *ServerConfig) SetDefaults() {
	if c.AppName == "" {
		c.AppName = "GoICHI"
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 60 * time.Second
	}
	if c.WriteTimeout == 0 {
		c.WriteTimeout = 60 * time.Second
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 120 * time.Second
	}
	if c.MaxRequestBodySize <= 0 {
		c.MaxRequestBodySize = 30 * 1024 * 1024 // 30MB
	}
}

// RoutingConfig tunes how request paths are matched.
type RoutingConfig struct {
	// StrictSlash treats "/users" and "/users/" as different routes.
	StrictSlash bool

	// CaseSensitive matches literal path segments exactly. When false (the
	// default) "/Users" also reaches a "/users" route; parameter values keep
	// their original casing either way.
	CaseSensitive bool
}

// Config is the application configuration passed to New.
type Config struct {
	Server  ServerConfig
	Routing RoutingConfig
	Docs    DocsConfig
	JWT     middleware.JWTConfig
	// Security is the default security scheme advertised in generated docs.
	Security openapi.SecurityScheme
}

// DocsConfig describes one generated OpenAPI reference page.
type DocsConfig struct {
	Enable      bool
	Path        string
	Title       string
	Desc        string
	PageTitle   string
	DarkMode    bool
	ScalarJSURL string
	Security    openapi.SecurityScheme
	Filter      func(*RouteInfo) bool
}

// App is a configured application: a router, a set of protocol servers and the
// middleware shared between them. Create one with New.
type App struct {
	Router      *Router
	Config      Config
	docsConfigs []DocsConfig
	onShutdown  []func()

	protocolMiddlewares []ProtocolFastHTTPMiddleware
	grpcUnary           []grpc.UnaryServerInterceptor
	grpcStream          []grpc.StreamServerInterceptor

	// Unified JWT configuration
	jwtConfig *middleware.JWTConfig
	Registry  *protocol.ProtocolRegistry
}

// New returns an App configured by config, with panic recovery and security
// headers already installed. When config.JWT.Secret is set, unified JWT auth is
// enabled for every protocol.
func New(config Config) *App {
	config.Server.SetDefaults()

	if config.Docs.Enable {
		if config.Docs.Path == "" {
			config.Docs.Path = "/docs"
		} else if config.Docs.Path[0] != '/' {
			config.Docs.Path = "/" + config.Docs.Path
		}
		if config.Docs.Title == "" {
			config.Docs.Title = config.Server.AppName + " API Reference"
		}
	}
	if config.Docs.PageTitle == "" {
		config.Docs.PageTitle = config.Server.AppName + " API Reference"
	}

	app := &App{
		Router:   NewRouter(config.Routing.StrictSlash, config.Routing.CaseSensitive),
		Config:   config,
		Registry: protocol.NewProtocolRegistry(),
	}

	if app.Config.Server.ShowStartup == nil {
		t := true
		app.Config.Server.ShowStartup = &t
	}

	// Recover first so it wraps every other middleware and the handler: an
	// unrecovered panic in fasthttp tears down the whole process, not just the
	// request. Security headers are on by default.
	app.Use(middleware.Recover())
	app.UseProtocol(MiddlewareAsProtocol(middleware.Recover()))
	app.UseAll(middleware.SecurityHeaders())
	if config.JWT.Secret != "" {
		app.SetJWTAuth(config.JWT)
	}

	return app
}

// SetJWTAuth installs one JWT configuration for every protocol. Routes marked
// with Route.NoAuth are exempt; so are the built-in docs, favicon and health
// endpoints.
func (a *App) SetJWTAuth(cfg middleware.JWTConfig) {
	if cfg.TokenLookup == "" {
		cfg.TokenLookup = "header:Authorization"
	}
	a.jwtConfig = &cfg

	// Registered separately from the global chain so NoAuth routes can skip it.
	a.Router.setAuthMiddleware(middleware.JWT(cfg))

	// Apply to other protocols
	a.GetProtocols().SetUnifiedAuth(cfg.Secret, cfg.TokenLookup)
}

// SetErrorHandler replaces the default handler for errors returned by routes.
func (a *App) SetErrorHandler(handler core.ErrorHandler) {
	a.Router.ErrorHandler = handler
}

// RegisterProtocol adds a protocol server to the app. It inherits the unified
// auth configuration and the middleware registered with UseProtocol.
func (a *App) RegisterProtocol(handler protocol.ProtocolHandler) {
	a.GetProtocols().Register(handler)

	// If unified auth is already set, apply it to the new handler
	if a.jwtConfig != nil {
		if aa, ok := handler.(protocol.AuthAware); ok {
			aa.SetAuthSecret(a.jwtConfig.Secret)
			aa.SetAuthLookup(a.jwtConfig.TokenLookup)
		}
	}

	a.registerProtocolHooks(handler)
}

// AddDocs registers an additional documentation page, letting one app expose
// several filtered views of its API.
func (a *App) AddDocs(config DocsConfig) {
	if config.Path == "" {
		config.Path = "/docs"
	} else if config.Path[0] != '/' {
		config.Path = "/" + config.Path
	}
	if config.Title == "" {
		config.Title = a.Config.Server.AppName + " API Reference"
	}
	if config.PageTitle == "" {
		config.PageTitle = a.Config.Server.AppName + " API Reference"
	}
	a.docsConfigs = append(a.docsConfigs, config)
}

// Use registers middleware for every route, including routes declared earlier.
func (a *App) Use(m ...Middleware) { a.Router.Use(m...) }

// UseAll registers middleware for the router and for every protocol server.
func (a *App) UseAll(m ...Middleware) {
	a.Use(m...)
	for _, mw := range m {
		a.UseProtocol(MiddlewareAsProtocol(mw))
	}
}

// GET registers a handler for GET requests to path.
func (a *App) GET(path string, h Handler, m ...Middleware) *Route {
	return a.Router.GET(path, h, m...)
}

// POST registers a handler for POST requests to path.
func (a *App) POST(path string, h Handler, m ...Middleware) *Route {
	return a.Router.POST(path, h, m...)
}

// PUT registers a handler for PUT requests to path.
func (a *App) PUT(path string, h Handler, m ...Middleware) *Route {
	return a.Router.PUT(path, h, m...)
}

// PATCH registers a handler for PATCH requests to path.
func (a *App) PATCH(path string, h Handler, m ...Middleware) *Route {
	return a.Router.PATCH(path, h, m...)
}

// DELETE registers a handler for DELETE requests to path.
func (a *App) DELETE(path string, h Handler, m ...Middleware) *Route {
	return a.Router.DELETE(path, h, m...)
}

// ANY registers the same handler for every common HTTP method.
func (a *App) ANY(path string, h Handler, m ...Middleware) Routes {
	return a.Router.ANY(path, h, m...)
}

// Group returns a route group sharing a path prefix and middleware.
func (a *App) Group(prefix string, m ...Middleware) *Group {
	if prefix == "" {
		prefix = "/"
	}
	if prefix[0] != '/' {
		prefix = "/" + prefix
	}
	return &Group{prefix: prefix, router: a.Router, middlewares: m}
}

// EnableHealthCheck registers a plain-text liveness endpoint. It is exempt from
// unified JWT auth so probes do not need a token.
func (a *App) EnableHealthCheck(path string) {
	if path == "" {
		path = "/health"
	}
	a.Router.GET(path, func(c *Context) error {
		c.RequestCtx.SetStatusCode(core.StatusOK)
		c.RequestCtx.SetBodyString("OK")
		return nil
	}).HideDoc().NoAuth()
}

// GetRoutes returns metadata for every registered route.
func (a *App) GetRoutes() []*RouteInfo { return a.Router.Routes }

// IsMaster reports whether this is the parent process, which is always true
// unless multiprocess mode spawned it as a child.
func (a *App) IsMaster() bool {
	return os.Getenv("GOICHI_CHILD") != "1"
}

// OnShutdown registers functions to run after the server stops accepting
// connections.
func (a *App) OnShutdown(f ...func()) {
	a.onShutdown = append(a.onShutdown, f...)
}

func (a *App) runShutdownHooks() {
	if len(a.onShutdown) == 0 {
		return
	}
	log.Print("🔄 Running shutdown hooks...")
	for _, f := range a.onShutdown {
		f()
	}
	log.Print("✅ All shutdown hooks completed")
}

// Listen binds addr and serves every registered protocol until the process
// receives SIGINT or SIGTERM.
func (a *App) Listen(addr string) error {
	if err := a.prepare(); err != nil {
		return err
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	var httpLn net.Listener
	var grpcLn net.Listener
	var mqttLn net.Listener
	var m cmux.CMux

	if a.Config.Server.EnablePortMultiplexing {
		m = cmux.New(ln)
		grpcLn = m.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
		mqttLn = m.Match(func(r io.Reader) bool {
			buf := make([]byte, 1)
			if _, err := io.ReadFull(r, buf); err != nil {
				return false
			}
			return buf[0] == 0x10
		})
		httpLn = m.Match(cmux.Any())

		// Inject listeners into protocols
		for _, h := range a.GetProtocols().GetAll() {
			switch h.GetName() {
			case "grpc":
				h.SetListener(grpcLn)
			case "mqtt":
				h.SetListener(mqttLn)
			}
		}
	} else {
		httpLn = ln
	}

	host := displayHost(addr, a.Config.Server.TLSEnabled())
	if *a.Config.Server.ShowStartup {
		log.Printf("🚀 %s listening on %s", a.Config.Server.AppName, host)
		if a.Config.Docs.Enable {
			log.Printf("📖 Docs: %s%s", host, a.Config.Docs.Path)
		}
		for _, doc := range a.docsConfigs {
			log.Printf("📖 Docs: %s%s", host, doc.Path)
		}
	}

	server := a.newServer()

	// Start all protocols
	go func() {
		if err := a.StartAllProtocols(context.Background()); err != nil {
			log.Printf("⚠️  Error starting protocols: %v", err)
		}
	}()

	errChan := make(chan error, 2)
	go func() {
		errChan <- a.serve(server, httpLn)
	}()

	if a.Config.Server.EnablePortMultiplexing {
		go func() {
			if err := m.Serve(); err != nil {
				errChan <- fmt.Errorf("cmux serve error: %w", err)
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		log.Printf("\n📡 Received signal: %v", sig)
		log.Print("🛑 Shutting down...")

		if m != nil {
			m.Close()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = a.StopAllProtocols(ctx)
		err := server.ShutdownWithContext(ctx)
		a.runShutdownHooks()
		return err
	}
}

// ListenGraceful binds addr like Listen, and additionally supports the
// multiprocess model configured by ServerConfig.Multiprocess.
func (a *App) ListenGraceful(addr string) error {
	if a.Config.Server.Multiprocess && os.Getenv("GOICHI_CHILD") != "1" {
		num := a.Config.Server.NumChildren
		if num <= 0 {
			num = runtime.NumCPU()
		}
		// Monitor child processes
		children := make(map[int]string) // PID -> dummy

		restartChild := func(pid int) {
			cmd := exec.Command(os.Args[0], os.Args[1:]...)
			cmd.Env = append(os.Environ(), "GOICHI_CHILD=1")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err == nil {
				children[cmd.Process.Pid] = "active"
				log.Printf("🔄 Restarted child process with PID: %d", cmd.Process.Pid)
			}
		}

		for i := 0; i < num; i++ {
			cmd := exec.Command(os.Args[0], os.Args[1:]...)
			cmd.Env = append(os.Environ(), "GOICHI_CHILD=1")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err == nil {
				children[cmd.Process.Pid] = "active"
			}
		}

		// Keep master process alive and monitor children
		startMonitor(children, restartChild)
		select {} // Keep master process alive
	}

	if err := a.prepare(); err != nil {
		return err
	}

	lc := getListenConfig(a.Config.Server.Multiprocess)
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	var httpLn net.Listener
	var grpcLn net.Listener
	var mqttLn net.Listener
	var m cmux.CMux

	if a.Config.Server.EnablePortMultiplexing {
		m = cmux.New(ln)
		grpcLn = m.MatchWithWriters(cmux.HTTP2MatchHeaderFieldSendSettings("content-type", "application/grpc"))
		mqttLn = m.Match(func(r io.Reader) bool {
			buf := make([]byte, 1)
			if _, err := io.ReadFull(r, buf); err != nil {
				return false
			}
			return buf[0] == 0x10
		})
		httpLn = m.Match(cmux.Any())

		// Inject listeners into protocols
		for _, h := range a.GetProtocols().GetAll() {
			switch h.GetName() {
			case "grpc":
				h.SetListener(grpcLn)
			case "mqtt":
				h.SetListener(mqttLn)
			}
		}

		go m.Serve()
	} else {
		httpLn = ln
	}

	host := displayHost(addr, a.Config.Server.TLSEnabled())

	if a.Config.Server.Multiprocess {
		// Child processes share the listener via SO_REUSEPORT; cmux demultiplexes
		// per process, so protocol traffic is not balanced across children.
		if a.Config.Server.EnablePortMultiplexing {
			log.Print("⚠️  Multiprocess with port multiplexing is not supported yet; protocols may be served by a single child")
		}
	}

	if *a.Config.Server.ShowStartup {
		banner := a.Config.Server.StartupBanner
		if banner == "" {
			banner = fmt.Sprintf("🚀 %s listening on %s", a.Config.Server.AppName, host)
		}
		log.Println(banner)

		if a.Config.Docs.Enable {
			log.Printf("📖 Docs: %s%s", host, a.Config.Docs.Path)
		}
		for _, doc := range a.docsConfigs {
			log.Printf("📖 Docs: %s%s", host, doc.Path)
		}
	}

	server := a.newServer()

	// Start all protocols
	go func() {
		if err := a.StartAllProtocols(context.Background()); err != nil {
			log.Printf("⚠️  Error starting protocols: %v", err)
		}
	}()

	errChan := make(chan error, 1)
	go func() {
		errChan <- a.serve(server, httpLn)
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		return err
	case sig := <-sigChan:
		log.Printf("\n📡 Received signal: %v", sig)
		log.Print("🛑 Shutting down...")

		if m != nil {
			m.Close()
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_ = a.StopAllProtocols(ctx)
		err := server.ShutdownWithContext(ctx)
		a.runShutdownHooks()

		if err != nil {
			log.Printf("⚠️  Error during shutdown: %v", err)
			return err
		}
		log.Print("✅ Server shutdown complete")
		return nil
	}
}

// prepare validates the configuration and finalises the route table. It runs
// before the listener is bound so misconfiguration fails fast.
func (a *App) prepare() error {
	if err := a.ValidateConfig(); err != nil {
		return err
	}
	a.registerFavicon()
	a.registerAllDocs()
	a.Router.Build()
	return nil
}

// newServer builds the fasthttp server for the main listener.
func (a *App) newServer() *fasthttp.Server {
	return &fasthttp.Server{
		Handler:            panicGuard(a.Router.HandleRequest),
		ReadTimeout:        a.Config.Server.ReadTimeout,
		WriteTimeout:       a.Config.Server.WriteTimeout,
		IdleTimeout:        a.Config.Server.IdleTimeout,
		MaxRequestBodySize: a.Config.Server.MaxRequestBodySize,
	}
}

// panicGuard is the last line of defence: fasthttp does not recover panics, so
// one escaping a handler would otherwise terminate the whole process.
func panicGuard(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] %s %s: %v\n%s", ctx.Method(), ctx.Path(), r, debug.Stack())
				ctx.ResetBody()
				ctx.SetStatusCode(core.StatusInternalServerError)
				_ = core.WriteJSONError(ctx, "internal server error")
			}
		}()
		next(ctx)
	}
}

// displayHost renders addr as a browsable URL for the startup banner.
func displayHost(addr string, tls bool) string {
	scheme := "http://"
	if tls {
		scheme = "https://"
	}
	switch {
	case addr == "":
		return scheme + "localhost"
	case addr[0] == ':':
		return scheme + "localhost" + addr
	default:
		return scheme + addr
	}
}

// serve runs the fasthttp server over ln, using TLS when a cert/key is configured.
func (a *App) serve(server *fasthttp.Server, ln net.Listener) error {
	if a.Config.Server.TLSEnabled() {
		return server.ServeTLS(ln, a.Config.Server.TLSCertFile, a.Config.Server.TLSKeyFile)
	}
	return server.Serve(ln)
}

func (a *App) registerFavicon() {
	const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="8" fill="#E24B4A"/>
  <text x="16" y="23" font-family="system-ui,sans-serif" font-size="20" font-weight="700"
        text-anchor="middle" fill="#ffffff">G</text>
</svg>`
	a.Router.GET("/favicon.ico", func(c *Context) error {
		c.RequestCtx.SetContentType("image/svg+xml")
		c.RequestCtx.Response.Header.Set("Cache-Control", "public, max-age=86400")
		c.RequestCtx.SetBodyString(faviconSVG)
		return nil
	}).HideDoc().NoAuth()
}

func (a *App) registerAllDocs() {
	if a.Config.Docs.Enable {
		a.registerSingleDoc(DocsConfig{
			Path:        a.Config.Docs.Path,
			Title:       a.Config.Docs.Title,
			Desc:        a.Config.Docs.Desc,
			PageTitle:   a.Config.Docs.PageTitle,
			DarkMode:    a.Config.Docs.DarkMode,
			ScalarJSURL: a.Config.Docs.ScalarJSURL,
			Security:    a.Config.Security,
			Filter:      nil,
		})
	}

	for _, config := range a.docsConfigs {
		a.registerSingleDoc(config)
	}
}

func (a *App) registerSingleDoc(config DocsConfig) {
	raw := make([]*openapi.RouteInfo, 0, len(a.Router.Routes))
	for _, r := range a.Router.Routes {
		if r.Path == "/favicon.ico" {
			continue
		}

		if r.HideDoc {
			continue
		}

		if config.Filter != nil && !config.Filter(r) {
			continue
		}

		raw = append(raw, &openapi.RouteInfo{
			Method:        r.Method,
			Path:          r.Path,
			Summary:       r.Summary,
			Tags:          r.Tags,
			ResponseModel: r.ResponseModel,
			RequestModel:  r.RequestModel,
			NoAuth:        r.NoAuth,
			HideDoc:       r.HideDoc,
			ContentType:   r.ContentType,
		})
	}
	description := config.Desc
	schema := openapi.GenerateSchema(config.Title, description, raw, config.Security)

	// Serve the OpenAPI schema as a JSON endpoint
	jsonPath := config.Path + "/openapi.json"
	a.Router.GET(jsonPath, func(c *Context) error {
		c.RequestCtx.SetContentType("application/json")
		c.RequestCtx.SetBodyString(schema)
		return nil
	}).HideDoc().NoAuth()

	// Use the JSON endpoint instead of a data URL for Scalar
	html := openapi.ScalarHTML(config.PageTitle, jsonPath, config.DarkMode, config.ScalarJSURL)

	a.Router.GET(config.Path, func(c *Context) error {
		c.RequestCtx.SetContentType("text/html")
		c.RequestCtx.SetBodyString(html)
		return nil
	}).HideDoc().NoAuth()
}
