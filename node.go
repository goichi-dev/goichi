package goichi

import (
	"strings"
)

type nodeType uint8

const (
	static nodeType = iota
	param
	catchAll
)

// node is a radix-tree node. A tree holds the routes of exactly one HTTP method.
type node struct {
	path     string
	nType    nodeType
	children []*node
	handler  Handler
	paramKey string
}

// insert adds handler at path. When ci is true, literal segments are matched
// case-insensitively, so "/Users" and "/users" collapse into one node.
func (n *node) insert(path string, handler Handler, ci bool) {
	if path == "" {
		n.handler = handler
		return
	}

	if path[0] == ':' {
		end := strings.IndexByte(path[1:], '/')
		var key, rest string
		if end == -1 {
			key = path[1:]
			rest = ""
		} else {
			key = path[1 : end+1]
			rest = path[end+1:]
		}

		for _, c := range n.children {
			if c.nType == param {
				c.insert(rest, handler, ci)
				return
			}
		}

		child := &node{nType: param, paramKey: key}
		n.children = append(n.children, child)
		child.insert(rest, handler, ci)
		return
	}

	if path[0] == '*' {
		key := path[1:]
		for _, c := range n.children {
			if c.nType == catchAll {
				// Last registration wins rather than being silently dropped.
				c.paramKey = key
				c.handler = handler
				return
			}
		}
		n.children = append(n.children, &node{
			nType:    catchAll,
			paramKey: key,
			handler:  handler,
		})
		return
	}

	// Only the leading literal run belongs to a static node. A ':' or '*' that
	// appears mid-path (e.g. "/hello/:name") starts a param/catchAll child and
	// must not be absorbed into the static path.
	staticPart, rest := path, ""
	if i := strings.IndexAny(path, ":*"); i > 0 {
		staticPart, rest = path[:i], path[i:]
	}

	for _, c := range n.children {
		if c.nType != static {
			continue
		}
		lcp := commonPrefix(c.path, staticPart, ci)
		if lcp == 0 {
			continue
		}

		if lcp == len(c.path) {
			c.insert(staticPart[lcp:]+rest, handler, ci)
			return
		}

		split := &node{
			path:     c.path[lcp:],
			nType:    static,
			children: c.children,
			handler:  c.handler,
		}
		c.path = c.path[:lcp]
		c.children = []*node{split}
		c.handler = nil
		c.insert(staticPart[lcp:]+rest, handler, ci)
		return
	}

	child := &node{path: staticPart, nType: static}
	n.children = append(n.children, child)
	if rest == "" {
		child.handler = handler
		return
	}
	child.insert(rest, handler, ci)
}

// capture is one parameter value bound during a walk.
type capture struct {
	key   string
	value string
}

// matchOrder is the priority in which children are tried: an exact literal
// beats a parameter, and a parameter beats a catch-all.
var matchOrder = [...]nodeType{static, param, catchAll}

// search walks the tree for path and returns the handler along with the
// parameters it bound. Parameter values keep their original casing even when
// ci is true. The returned map is nil for routes that take no parameters.
func (n *node) search(path string, ci bool) (Handler, map[string]string) {
	// Sized to cover any realistic route; a deeper one simply grows onto the
	// heap. The array itself does not escape, so the common case allocates
	// nothing at all.
	var scratch [8]capture

	handler, caps := n.match(path, scratch[:0], ci)
	if handler == nil || len(caps) == 0 {
		return handler, nil
	}

	params := make(map[string]string, len(caps))
	for _, c := range caps {
		params[c.key] = c.value
	}
	return handler, params
}

// match is the recursive half of search. Bound parameters are appended to caps
// and a failed branch is undone by truncating back to the length recorded
// before it was tried, so no per-branch copy is needed.
func (n *node) match(path string, caps []capture, ci bool) (Handler, []capture) {
	switch n.nType {
	case static:
		if !hasPrefix(path, n.path, ci) {
			return nil, caps
		}
		path = path[len(n.path):]

	case param:
		end := strings.IndexByte(path, '/')
		var value string
		if end == -1 {
			value = path
			path = ""
		} else {
			value = path[:end]
			path = path[end:]
		}
		if value == "" {
			return nil, caps
		}
		caps = append(caps, capture{n.paramKey, value})

	case catchAll:
		return n.handler, append(caps, capture{n.paramKey, path})
	}

	if path == "" {
		if n.handler != nil {
			return n.handler, caps
		}
		// A bare catch-all child still matches an empty remainder.
		for _, child := range n.children {
			if child.nType == catchAll && child.handler != nil {
				return child.handler, append(caps, capture{child.paramKey, ""})
			}
		}
		return nil, caps
	}

	mark := len(caps)
	for _, want := range matchOrder {
		for _, child := range n.children {
			if child.nType != want {
				continue
			}
			if h, c := child.match(path, caps[:mark], ci); h != nil {
				return h, c
			}
		}
	}

	return nil, caps
}

// hasPrefix reports whether s starts with prefix, folding ASCII case when ci.
func hasPrefix(s, prefix string, ci bool) bool {
	if len(s) < len(prefix) {
		return false
	}
	if !ci {
		return s[:len(prefix)] == prefix
	}
	for i := 0; i < len(prefix); i++ {
		if lowerASCII(s[i]) != lowerASCII(prefix[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// commonPrefix returns the length of the shared leading run of a and b.
func commonPrefix(a, b string, ci bool) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		x, y := a[i], b[i]
		if ci {
			x, y = lowerASCII(x), lowerASCII(y)
		}
		if x != y {
			return i
		}
	}
	return n
}
