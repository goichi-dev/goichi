package goichi

import (
	"encoding/base64"
	"log"
	"slices"
	"strings"
	"sync"

	"github.com/goichi-dev/goichi/core"
	"github.com/valyala/fasthttp"
)

// Router matches incoming requests against a radix tree per HTTP method and
// invokes the registered handler.
//
// Handlers are not composed with their middleware at registration time. The
// whole table is compiled once, on the first request or on an explicit Build
// call, so that middleware registered with Use after a route was declared still
// applies to it.
type Router struct {
	trees       map[string]*node
	middlewares []Middleware

	// authMiddleware is kept out of middlewares so that routes marked NoAuth
	// can opt out of it while still receiving every other global middleware.
	authMiddleware Middleware

	entries []*routeEntry

	// Routes describes every registered route. It backs the generated OpenAPI
	// document and is safe to read after registration.
	Routes []*RouteInfo

	// ErrorHandler, when set, receives any error returned by a handler.
	ErrorHandler core.ErrorHandler

	// StrictSlash distinguishes "/users" from "/users/" when true. When false
	// (the default) a trailing slash is ignored.
	StrictSlash bool

	// CaseSensitive matches literal path segments exactly when true. When false
	// (the default) "/Users" also matches a "/users" route. Parameter values
	// always keep their original casing.
	CaseSensitive bool

	buildOnce sync.Once
	fallback  *fallbackHandlers
}

// routeEntry is a route as declared by the caller, before middleware is applied.
type routeEntry struct {
	method  string
	path    string
	handler Handler
	mws     []Middleware
	info    *RouteInfo
}

// fallbackHandlers hold the compiled chains used when no route matches, so that
// global middleware (CORS, logging, security headers) still runs on 404, 405 and
// automatic OPTIONS responses.
type fallbackHandlers struct {
	notFound   Handler
	notAllowed Handler
	options    Handler
}

// RouteInfo is the metadata describing a single registered route.
type RouteInfo struct {
	Method        string
	Path          string
	Summary       string
	Tags          []string
	ResponseModel any
	RequestModel  any
	NoAuth        bool
	HideDoc       bool
	ContentType   string
}

// NewRouter returns a Router. See Router.StrictSlash and Router.CaseSensitive
// for the meaning of the two flags.
func NewRouter(strictSlash, caseSensitive bool) *Router {
	return &Router{
		trees:         make(map[string]*node),
		StrictSlash:   strictSlash,
		CaseSensitive: caseSensitive,
	}
}

// Use registers global middleware. Unlike most routers, Use may be called after
// routes have been declared; the middleware still applies to them.
func (r *Router) Use(m ...Middleware) {
	r.middlewares = append(r.middlewares, m...)
}

// setAuthMiddleware installs the middleware that routes marked NoAuth skip.
func (r *Router) setAuthMiddleware(m Middleware) {
	r.authMiddleware = m
}

// GET registers a handler for GET requests to path.
func (r *Router) GET(path string, h Handler, m ...Middleware) *Route {
	return r.addRoute("GET", path, h, m...)
}

// POST registers a handler for POST requests to path.
func (r *Router) POST(path string, h Handler, m ...Middleware) *Route {
	return r.addRoute("POST", path, h, m...)
}

// PUT registers a handler for PUT requests to path.
func (r *Router) PUT(path string, h Handler, m ...Middleware) *Route {
	return r.addRoute("PUT", path, h, m...)
}

// PATCH registers a handler for PATCH requests to path.
func (r *Router) PATCH(path string, h Handler, m ...Middleware) *Route {
	return r.addRoute("PATCH", path, h, m...)
}

// DELETE registers a handler for DELETE requests to path.
func (r *Router) DELETE(path string, h Handler, m ...Middleware) *Route {
	return r.addRoute("DELETE", path, h, m...)
}

// ANY registers the same handler for every common HTTP method.
func (r *Router) ANY(path string, h Handler, m ...Middleware) Routes {
	routes := make(Routes, len(anyMethods))
	for i, method := range anyMethods {
		routes[i] = r.addRoute(method, path, h, m...)
	}
	return routes
}

var anyMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}

func (r *Router) addRoute(method, path string, h Handler, m ...Middleware) *Route {
	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}

	info := &RouteInfo{Method: method, Path: path}
	r.Routes = append(r.Routes, info)

	mws := make([]Middleware, len(m))
	copy(mws, m)

	r.entries = append(r.entries, &routeEntry{
		method:  method,
		path:    path,
		handler: h,
		mws:     mws,
		info:    info,
	})
	return newRoute(info)
}

// Build compiles the route table. It is called automatically before the first
// request; calling it earlier surfaces registration problems at startup.
func (r *Router) Build() {
	r.buildOnce.Do(r.build)
}

func (r *Router) build() {
	r.trees = make(map[string]*node, len(r.trees))

	for _, e := range r.entries {
		chain := make([]Middleware, 0, len(r.middlewares)+len(e.mws)+1)
		chain = append(chain, r.middlewares...)
		if r.authMiddleware != nil && !e.info.NoAuth {
			chain = append(chain, r.authMiddleware)
		}
		chain = append(chain, e.mws...)

		handler := e.handler
		for i := len(chain) - 1; i >= 0; i-- {
			handler = chain[i](handler)
		}

		if r.trees[e.method] == nil {
			r.trees[e.method] = &node{path: "", nType: static}
		}
		path := r.matchPath(e.path)
		if path == "/" {
			r.trees[e.method].handler = handler
		} else {
			r.trees[e.method].insert(path[1:], handler, r.foldCase())
		}
	}

	r.fallback = r.buildFallbacks()
}

// buildFallbacks compiles the 404 / 405 / OPTIONS responders through the global
// middleware chain, so a rejected request still gets CORS and security headers.
func (r *Router) buildFallbacks() *fallbackHandlers {
	wrap := func(h Handler) Handler {
		for i := len(r.middlewares) - 1; i >= 0; i-- {
			h = r.middlewares[i](h)
		}
		return h
	}

	return &fallbackHandlers{
		notFound: wrap(func(c *Context) error {
			c.RequestCtx.SetStatusCode(core.StatusNotFound)
			return core.WriteJSONError(c.RequestCtx, "not found")
		}),
		notAllowed: wrap(func(c *Context) error {
			setAllowHeader(c)
			c.RequestCtx.SetStatusCode(core.StatusMethodNotAllowed)
			return core.WriteJSONError(c.RequestCtx, "method not allowed")
		}),
		options: wrap(func(c *Context) error {
			setAllowHeader(c)
			c.RequestCtx.SetStatusCode(core.StatusNoContent)
			return nil
		}),
	}
}

func setAllowHeader(c *Context) {
	if allow, ok := c.Get(allowLocalsKey).(string); ok && allow != "" {
		c.RequestCtx.Response.Header.Set("Allow", allow)
	}
}

const allowLocalsKey = "goichi.allow"

// foldCase reports whether literal segments should be matched ignoring ASCII
// case. It is the inverse of CaseSensitive.
func (r *Router) foldCase() bool { return !r.CaseSensitive }

// matchPath normalises a path for matching according to StrictSlash and
// CaseSensitive. Casing is handled during the tree walk so that parameter
// values keep their original form; only the trailing slash is rewritten here.
func (r *Router) matchPath(path string) string {
	if !r.StrictSlash && len(path) > 1 && path[len(path)-1] == '/' {
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "/"
		}
	}
	return path
}

// allowedMethods lists the methods that have a route matching path.
func (r *Router) allowedMethods(path string) []string {
	var allowed []string
	for method, root := range r.trees {
		if root == nil {
			continue
		}
		if path == "/" {
			if root.handler != nil {
				allowed = append(allowed, method)
			}
			continue
		}
		if h, _ := root.search(path[1:], nil, r.foldCase()); h != nil {
			allowed = append(allowed, method)
		}
	}
	slices.Sort(allowed)
	return allowed
}

// HandleRequest is the fasthttp entry point for the router.
func (r *Router) HandleRequest(ctx *fasthttp.RequestCtx) {
	r.buildOnce.Do(r.build)

	method := string(ctx.Method())
	path := r.matchPath(string(ctx.Path()))
	if path == "" {
		path = "/"
	}

	if root := r.trees[method]; root != nil {
		var handler Handler
		var params map[string]string
		if path == "/" {
			handler = root.handler
		} else {
			handler, params = root.search(path[1:], nil, r.foldCase())
		}
		if handler != nil {
			r.run(ctx, handler, params, nil)
			return
		}
	}

	// The path exists under other methods: answer OPTIONS ourselves and return
	// 405 (with an Allow header) instead of a misleading 404.
	if allowed := r.allowedMethods(path); len(allowed) > 0 {
		if method == "OPTIONS" {
			allowed = appendIfMissing(allowed, "OPTIONS")
			r.run(ctx, r.fallback.options, nil, allowed)
			return
		}
		if method == "HEAD" && slices.Contains(allowed, "GET") {
			if root := r.trees["GET"]; root != nil {
				var handler Handler
				var params map[string]string
				if path == "/" {
					handler = root.handler
				} else {
					handler, params = root.search(path[1:], nil, r.foldCase())
				}
				if handler != nil {
					r.run(ctx, handler, params, nil)
					ctx.Response.ResetBody()
					return
				}
			}
		}
		allowed = appendIfMissing(allowed, "OPTIONS")
		r.run(ctx, r.fallback.notAllowed, nil, allowed)
		return
	}

	r.run(ctx, r.fallback.notFound, nil, nil)
}

func (r *Router) run(ctx *fasthttp.RequestCtx, handler Handler, params map[string]string, allowed []string) {
	c := &Context{RequestCtx: ctx, Params: params}
	if len(allowed) > 0 {
		c.Set(allowLocalsKey, strings.Join(allowed, ", "))
	}
	if err := handler(c); err != nil {
		r.handleError(c, err)
	}
}

func (r *Router) handleError(c *Context, err error) {
	if r.ErrorHandler != nil {
		_ = r.ErrorHandler(c, err)
		return
	}
	ctx := c.RequestCtx
	// Never leak the raw error to clients; log it server-side and return a
	// generic message (validation errors are safe to show).
	if ve, ok := err.(*core.ValidationError); ok {
		ctx.SetStatusCode(core.StatusUnprocessableEntity)
		_ = core.WriteJSONError(ctx, ve.Error())
		return
	}
	log.Printf("[ERROR] %s %s: %v", ctx.Method(), ctx.Path(), err)
	ctx.SetStatusCode(core.StatusInternalServerError)
	_ = core.WriteJSONError(ctx, "internal server error")
}

func appendIfMissing(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// EnableDocs serves a static HTML page at path.
func (r *Router) EnableDocs(path string, htmlContent string) {
	r.GET(path, func(c *Context) error {
		c.RequestCtx.SetContentType("text/html")
		c.RequestCtx.SetBodyString(htmlContent)
		return nil
	})
}

// Static serves the files under root at the given URL prefix.
func (r *Router) Static(prefix, root string) {
	if prefix == "" {
		prefix = "/"
	}
	if prefix[0] != '/' {
		prefix = "/" + prefix
	}

	stripPrefix := prefix
	if stripPrefix[len(stripPrefix)-1] != '/' {
		stripPrefix += "/"
	}

	fs := &fasthttp.FS{
		Root:               root,
		IndexNames:         []string{"index.html"},
		GenerateIndexPages: false,
		Compress:           false,
		AcceptByteRange:    true,
		PathRewrite:        fasthttp.NewPathPrefixStripper(len(stripPrefix) - 1),
	}
	h := fs.NewRequestHandler()

	routePath := prefix
	if routePath[len(routePath)-1] == '/' {
		routePath += "*filepath"
	} else {
		routePath += "/*filepath"
	}

	r.GET(routePath, func(c *Context) error {
		h(c.RequestCtx)
		return nil
	}).HideDoc()
}

// B64Encode returns the standard base64 encoding of s.
func B64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
