# Goichi

[![Go Reference](https://pkg.go.dev/badge/github.com/goichi-dev/goichi.svg)](https://pkg.go.dev/github.com/goichi-dev/goichi)
[![Go Report Card](https://goreportcard.com/badge/github.com/goichi-dev/goichi)](https://goreportcard.com/report/github.com/goichi-dev/goichi)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Goichi is a high-performance, modular web API framework for Go with multi-protocol
support. A single application can serve REST, WebSocket, GraphQL, gRPC/ConnectRPC,
MQTT and MCP — through one unified middleware and authentication model.

> **Status: v0.x.** The API may still change between minor versions.

## Features

- **One app, many protocols** — REST, WebSocket, GraphQL, gRPC, ConnectRPC, MQTT and MCP, optionally multiplexed onto a single port.
- **Unified middleware** — the same middleware chain (auth, logging, rate limiting, recovery, CORS, …) applies across protocols.
- **Unified JWT auth** — configure a signing secret once and every protocol enforces it. Built on `github.com/golang-jwt/jwt/v5`, pinned to HMAC and requiring an expiry.
- **Secure by default** — panic recovery and security headers are installed for you, CORS allows no origins until you configure it, and errors never leak internals to clients.
- **Correct HTTP by default** — automatic `OPTIONS` (so CORS preflight works without extra routes), `405` with an `Allow` header, and `HEAD` served from the matching `GET`.
- **Built-in API docs** — generate OpenAPI documentation from your routes.

## Install

```bash
go get github.com/goichi-dev/goichi
```

Requires Go 1.25 or later.

## Quick Start

```go
package main

import "github.com/goichi-dev/goichi"

func main() {
	app := goichi.New(goichi.Config{})

	app.GET("/hello/:name", func(c *goichi.Context) error {
		return c.JSON(map[string]string{
			"message": "Hello, " + c.Param("name"),
		})
	})

	app.Listen(":3000")
}
```

```bash
go run .
# GET http://localhost:3000/hello/world -> {"message":"Hello, world"}
```

## Routing

Paths support `:name` parameters and a trailing `*rest` catch-all. Static segments
take priority over parameters, which take priority over the catch-all.

By default a trailing slash is ignored and literal segments match without regard
to ASCII case — `/Users/42` reaches a `/users/:id` route. Parameter values always
keep their original casing. Both behaviours are configurable:

```go
app := goichi.New(goichi.Config{
	Routing: goichi.RoutingConfig{StrictSlash: true, CaseSensitive: true},
})
```

## Middleware

Middleware is a `func(goichi.Handler) goichi.Handler`. Register it globally, per
group, or per route:

```go
app.Use(middleware.Logger())

api := app.Group("/api", middleware.RateLimit(100))
api.GET("/users", listUsers, middleware.Cache(30*time.Second))
```

The route table is compiled once before the first request, so `Use` still applies
to routes that were declared before it.

## Authentication

One secret protects every protocol:

```go
app := goichi.New(goichi.Config{
	JWT: middleware.JWTConfig{Secret: os.Getenv("JWT_SECRET")},
})

app.GET("/public", handler).NoAuth() // opt out
```

The generated docs, `/favicon.ico` and the health endpoint are exempt already, so
enabling auth never locks you out of your own API reference.

## Multi-protocol example

Runnable examples — one per protocol, plus a server that starts every protocol
from a single process — live in
[goichi-dev/goichi-examples](https://github.com/goichi-dev/goichi-examples).

## Documentation

- [pkg.go.dev reference](https://pkg.go.dev/github.com/goichi-dev/goichi)
- [Multi-Protocol Support Guide](PROTOCOL_GUIDE.md) — per-protocol configuration,
  authentication details, and the production checklist.

## Contributing

Bug reports and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md)
for the build and review process, and [SECURITY.md](SECURITY.md) for reporting a
vulnerability privately.

## License

MIT — see [LICENSE](LICENSE).
