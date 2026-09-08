# Goichi Multi-Protocol Support - Guide

Goichi supports WebSocket, GraphQL, gRPC, MQTT and MCP (5 protocols), plus a TCP
reverse proxy that fronts any byte-stream service. This guide covers quick-start,
configuration examples, and production recommendations.

---

## Protocols Overview

| Protocol | Typical Port | Use Case |
|----------|--------------|----------|
| WebSocket | 8080 | Real-time bidirectional communication |
| GraphQL | 8081 | Flexible queries, aggregation, subscriptions |
| gRPC | 8082 | High-performance RPC using HTTP/2 and protobuf |
| MQTT | 1883 | Lightweight IoT pub/sub broker |
| MCP | 8083 | AI tool/context protocol for agents |
| TCP proxy | any | Layer-4 reverse proxy for Postgres, MySQL, Redis, SMTP |

---

## Quick Start

A runnable server that starts every protocol from a single process is maintained
in [goichi-dev/goichi-examples](https://github.com/goichi-dev/goichi-examples),
alongside one example per protocol.

It serves REST, gRPC, ConnectRPC, GraphQL, MQTT and WebSocket on one multiplexed
port (`127.0.0.1:8080`), with MCP on `:8085`. Open
http://127.0.0.1:8080/docs for the generated API documentation.

---

## Security & Behavior Defaults

- `goichi.New(goichi.Config{})` installs the Recover and SecurityHeaders middleware by default. SecurityHeaders sets HSTS, CSP, X-Frame-Options, X-Content-Type-Options and Referrer-Policy. Recover is not optional in practice: fasthttp does not recover panics itself, so without it a panic in any handler terminates the whole process.
- JWT auth uses the official `github.com/golang-jwt/jwt/v5` library. Signing is pinned to HMAC (HS256/384/512) and a valid `exp` claim is required. The `EncodeJWT` / `EncodeJWTWithTTL` / `DecodeJWT` helpers are available for convenience; `EncodeJWT` adds a one-hour `exp` when the claims do not carry one.
- CORS is conservative by default: `AllowOrigins` is empty, so you must set explicit allowed origins for browser clients. A `Vary: Origin` header is always emitted.
- The cache middleware skips responses that carry an Authorization header, so per-user data is never served from a shared cache.
- The recover middleware logs panics with a stack trace server-side but returns a generic "internal server error" to clients — no stack trace is exposed. Router and protocol error paths behave the same way: a generic message to the client, the real error logged server-side.
- The router answers `OPTIONS` automatically and returns `405` with an `Allow` header for a known path used with the wrong method, so CORS preflight works without declaring `OPTIONS` routes. `HEAD` is served from the matching `GET` route with the body stripped.
- The unified JWT chain skips the generated docs, `/favicon.ico` and the health endpoint; mark any other public route with `.NoAuth()`.
- MCP speaks JSON-RPC over a raw TCP socket, not HTTP. It always listens on its own port and is never mounted on the shared HTTP router, so it has no URL path and cannot be reached with `curl`.
- The app exposes protocol registry helpers: `InitProtocols`, `GetProtocols`, `StartAllProtocols`, `StopAllProtocols` and `RegisterProtocol`.
- The TCP proxy (`proxy.NewTCPProxy`) is a protocol server like any other: register it with `RegisterProtocol` and it starts and stops with the app. It forwards bytes and does not parse them, so it carries no JWT enforcement of its own — authentication stays with the upstream service, and `OnConnect` is the place for connection-level policy such as an IP allow-list.

### Unified authentication across protocols

Configure a JWT secret once via `SetJWTAuth` / `Config.JWT`, and every protocol enforces it:

- **WebSocket** — the JWT is validated on the upgrade request (per `TokenLookup`; use `query:token` for browsers). `AllowedOrigins` defends against Cross-Site WebSocket Hijacking (same-origin by default), and `MaxConnections` is enforced.
- **GraphQL** — queries require a valid JWT. The playground and schema introspection are **disabled by default** (`EnablePlayground` / `EnableIntrospection`). `MaxComplexity` limits expensive/nested queries.
- **gRPC / ConnectRPC** — validated via interceptor / HTTP middleware. `MaxConcurrentStreams` and keepalive are applied.
- **MQTT** — a single auth hook governs CONNECT: with a secret set, the CONNECT password must be a valid JWT. Anonymous connections are allowed only when `AllowAnonymous` is set and `AuthEnabled` is false.
- **MCP** — clients must authenticate in the `initialize` call (`params.token`); all other methods are rejected until then. Requests are size-capped (`MaxRequestBytes`) and idle-timeout bounded.
- **TCP proxy** — not covered by unified auth: it is a byte pipe, and the upstream service (Postgres, Redis, …) performs its own authentication. Use `OnConnect` to reject a connection before an upstream is dialled.

### Built-in TLS

Each protocol config (and `ServerConfig`) accepts `TLSCertFile` / `TLSKeyFile`. When both are set, the protocol serves over TLS (https / wss / grpcs / mqtts) directly; you can also terminate TLS upstream instead.

---

## Production Checklist (recommended)

- [ ] TLS/SSL: Terminate TLS (or enable on servers) for WebSocket (wss), gRPC (TLS) and MQTT (TLS).
- [ ] Configure CORS with explicit allowed origins; avoid wildcard "*" in production.
- [ ] Provide a strong JWT secret; the server will error if missing.
- [ ] Enable rate-limiting and request body limits.
- [ ] Ensure secure logging (do not log Authorization or PII).
- [ ] Use reverse-proxy headers (X-Forwarded-For) if behind load balancers for correct IP-based rate-limiting.
- [ ] Review cache rules to avoid caching authenticated responses.
- [ ] Add monitoring and health-check endpoints.
- [ ] For a TCP proxy in front of a database, set `MaxConnections` and `IdleTimeout` so a client cannot exhaust the upstream's connection slots, and enable `HealthCheck` so a failed replica leaves the pool.
- [ ] Do not expose a TCP proxy to an untrusted network on the assumption that the proxy authenticates: it does not. Restrict it at the network layer or in `OnConnect`.

---

For code examples and protocol-specific configuration, see the `protocol/*` subpackages in the repository.
