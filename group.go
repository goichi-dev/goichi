package goichi

import "strings"

type Group struct {
	prefix      string
	router      *Router
	middlewares []Middleware
}

// Group returns a nested group whose prefix and middleware extend this one.
func (g *Group) Group(prefix string, m ...Middleware) *Group {
	parent := strings.TrimSuffix(g.prefix, "/")
	child := "/" + strings.TrimPrefix(prefix, "/")
	return &Group{
		prefix: parent + child,
		router: g.router,
		// merge copies into a fresh slice: appending directly to g.middlewares
		// would let sibling groups share a backing array and overwrite each
		// other's middleware.
		middlewares: g.merge(m),
	}
}

func (g *Group) GET(path string, h Handler, m ...Middleware) *Route {
	return g.router.addRoute("GET", g.fullPath(path), h, g.merge(m)...)
}

func (g *Group) POST(path string, h Handler, m ...Middleware) *Route {
	return g.router.addRoute("POST", g.fullPath(path), h, g.merge(m)...)
}

func (g *Group) PUT(path string, h Handler, m ...Middleware) *Route {
	return g.router.addRoute("PUT", g.fullPath(path), h, g.merge(m)...)
}

func (g *Group) PATCH(path string, h Handler, m ...Middleware) *Route {
	return g.router.addRoute("PATCH", g.fullPath(path), h, g.merge(m)...)
}

func (g *Group) DELETE(path string, h Handler, m ...Middleware) *Route {
	return g.router.addRoute("DELETE", g.fullPath(path), h, g.merge(m)...)
}

func (g *Group) ANY(path string, h Handler, m ...Middleware) Routes {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	routes := make(Routes, len(methods))
	for i, method := range methods {
		routes[i] = g.router.addRoute(method, g.fullPath(path), h, g.merge(m)...)
	}
	return routes
}

func (g *Group) Static(prefix, root string) {
	g.router.Static(g.prefix+prefix, root)
}

func (g *Group) fullPath(path string) string {
	prefix := strings.TrimSuffix(g.prefix, "/")
	if path == "" || path == "/" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}
	p := "/" + strings.TrimPrefix(path, "/")
	return prefix + p
}

func (g *Group) merge(m []Middleware) []Middleware {
	merged := make([]Middleware, 0, len(g.middlewares)+len(m))
	merged = append(merged, g.middlewares...)
	return append(merged, m...)
}
