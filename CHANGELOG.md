# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

While the version is `v0.x`, breaking changes may land in any minor release.

## [Unreleased]

First public release preparation. Everything below is relative to the private
pre-release code, so no upgrade path is documented — there was no published
version to upgrade from.

### Fixed

- **A panic in any handler no longer terminates the process.** fasthttp does not
  recover panics, so a single `panic` in one request killed the connection and
  then the entire server. `Recover` is now installed by default in `New`, and a
  second guard wraps the fasthttp handler itself. A panic returns `500` and the
  server keeps serving.
- **`middleware.JWT` no longer panics on a malformed `TokenLookup`.** A value
  without a `source:key` separator (for example `"Authorization"`) caused an
  index-out-of-range panic. It now falls back to `header:Authorization`. Token
  extraction is shared with `ExtractToken`, so the `cookie:` source works in the
  middleware too.
- **`EncodeJWT` and `ValidateJWT` now round-trip.** `ValidateJWT` requires an
  `exp` claim, but `EncodeJWT` never set one, so a token minted by the framework
  was rejected by the framework. `EncodeJWT` now adds `exp` (and `iat`) when the
  claims do not carry them.
- **CORS preflight works.** An `OPTIONS` request to a path with no `OPTIONS`
  route returned `404` and never reached the CORS middleware. The router now
  answers `OPTIONS` itself, through the global middleware chain.
- **Wrong method returns `405`, not `404`**, together with an `Allow` header.
- **`HEAD` is served from the matching `GET` route** with the body stripped.
- **Enabling `Config.JWT.Secret` no longer locks the built-in endpoints.** The
  generated docs, `openapi.json`, `/favicon.ico` and the health endpoint were all
  returning `401`, with no way to opt out. They are exempt now, and
  `Route.NoAuth()` exempts any other route.
- **`Use` after a route was declared now applies to that route.** Middleware used
  to be baked into the handler at registration time, so later `Use` calls were
  silently ignored. The route table is compiled once, before the first request.
- **Global middleware no longer runs twice on the main HTTP path.** The protocol
  middleware chain was applied to the router's own server as well as to protocol
  servers, so `UseAll(Logger())` logged every request twice and
  `UseAll(RateLimit(n))` counted each request twice.
- **Nested groups no longer overwrite each other's middleware.** Sibling groups
  created from the same parent could share a slice backing array, so one group
  silently received another's middleware — including auth.
- **`Routing.StrictSlash` and `Routing.CaseSensitive` are implemented.** They were
  stored and never read. Case-insensitive matching folds ASCII case on literal
  segments only; parameter values keep their original casing.
- **`MQTTBroker.Subscribe` works.** It was an empty stub that silently discarded
  the handler. It now registers an inline subscription, supports the `+` and `#`
  wildcards, and is paired with `Unsubscribe`.
- **A duplicate catch-all route no longer silently discards the new handler.**
- **`Listen("")` and `ListenGraceful("")` no longer panic** while formatting the
  startup banner.
- **The JWT error handler no longer builds JSON by string concatenation**, which
  could corrupt the response body.
- **Startup no longer logs one line per route.** `[Docs] Inspecting route: …` was
  unconditional debug output.
- **`validate:"min="` / `"max="` now apply to slices, arrays and maps** as a
  length check, instead of being ignored.

### Added

- `App.ValidateConfig` runs automatically from `Listen` and `ListenGraceful`, so
  misconfiguration fails before the listener is bound.
- `Router.Build` compiles the route table explicitly, for callers that want
  registration errors surfaced at startup.
- `middleware.EncodeJWTWithTTL` and `middleware.DefaultTokenTTL`.
- `MQTTBroker.Unsubscribe`.
- Package documentation, plus doc comments across the exported API, so
  pkg.go.dev renders a usable reference.
- `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, this changelog, GitHub
  Actions CI, Dependabot and issue templates.

### Changed

- **Module path is now `github.com/goichi-dev/goichi`.** The previous path,
  `github.com/goichi/goichi`, could never resolve: `github.com/goichi` is an
  unrelated GitHub account, so `go get` on that path was impossible.
- **Minimum Go version lowered from 1.26.4 to 1.25.0**, the floor the
  dependencies allow.
- **`MQTTBroker.Subscribe` signature changed** to
  `Subscribe(topic string, handler func(topic string, payload []byte)) error`.
  The `qos` parameter was removed because inline broker subscriptions do not
  negotiate QoS, and the method now reports failures instead of returning
  nothing.
- **404 and 405 response bodies** are `{"error":"not found"}` and
  `{"error":"method not allowed"}`.
- Status-code constants and their comments are in English.

### Removed

- **The `cmd/goichi` scaffolding CLI.** It generated a broken `.air.toml` (its
  build command targeted a library root), scaffolded a gqlgen setup the framework
  does not require, ignored every I/O error, and — most importantly — forced
  `spf13/cobra` and `spf13/pflag` into the dependency graph of every application
  that imported the library. Runnable code belongs in
  [goichi-examples](https://github.com/goichi-dev/goichi-examples).
- **Dead configuration fields on `MCPConfig`:** `Path`, `SessionPath`,
  `AllowClientSSE` and `VersionRequired` were advertised but never read. MCP
  speaks JSON-RPC over raw TCP and is never mounted on the HTTP router, so there
  is no path to configure.
