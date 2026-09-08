// Package goichi is a multi-protocol web API framework for Go.
//
// A single App can serve REST, WebSocket, GraphQL, gRPC, ConnectRPC, MQTT and
// MCP, optionally multiplexed onto one TCP port, behind one middleware chain and
// one JWT configuration.
//
// # Getting started
//
//	app := goichi.New(goichi.Config{})
//
//	app.GET("/hello/:name", func(c *goichi.Context) error {
//		return c.JSON(map[string]string{"message": "Hello, " + c.Param("name")})
//	})
//
//	app.Listen(":3000")
//
// # Routing
//
// Paths may contain ":name" parameters and a trailing "*rest" catch-all. Static
// segments win over parameters, which win over the catch-all. By default the
// router ignores a trailing slash and matches literal segments without regard to
// ASCII case; see [RoutingConfig]. Parameter values always keep their original
// casing.
//
// Requests to a known path with an unregistered method receive 405 together with
// an Allow header, and OPTIONS is answered automatically, so CORS preflight works
// without declaring OPTIONS routes.
//
// # Middleware
//
// Middleware is a func(Handler) Handler. Register it globally with App.Use, per
// group, or per route. The route table is compiled once before the first request,
// so Use may be called after the routes it should wrap have been declared.
//
// [middleware.Recover] and [middleware.SecurityHeaders] are installed by default:
// fasthttp does not recover panics on its own, and an escaping panic would take
// down the process rather than the request.
//
// # Authentication
//
// Setting Config.JWT.Secret enables the same HS256 verification for every
// protocol at once. Individual routes opt out with Route.NoAuth; the built-in
// docs, favicon and health endpoints already do.
//
// # Protocols
//
// Protocol servers are registered with App.RegisterProtocol and started with the
// app. With Config.Server.EnablePortMultiplexing the HTTP-based protocols share
// the main port. MQTT and MCP always listen on their own port: MCP speaks
// JSON-RPC over raw TCP and is never mounted on the HTTP router.
//
// # Reverse proxying
//
// The proxy subpackage forwards routes to upstream services over HTTP, with
// load balancing, health checks and WebSocket passthrough, and also provides a
// layer-4 TCP proxy for byte-stream services such as Postgres or Redis. The TCP
// proxy is itself a protocol server, so it starts and stops with the app.
package goichi
