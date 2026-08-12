# Security Policy

## Supported versions

Goichi is on `v0.x`. Only the most recent release receives security fixes.

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it privately through GitHub's
[private vulnerability reporting](https://github.com/goichi-dev/goichi/security/advisories/new)
on this repository. Include:

- the affected version or commit,
- a description of the impact,
- the smallest program or request sequence that reproduces it.

You can expect an acknowledgement within seven days, and an assessment with a
planned fix or an explanation within thirty days. Please give us a chance to
release a fix before disclosing publicly.

## Scope

Goichi handles authentication and terminates network protocols, so the following
are always in scope:

- bypassing JWT verification on any protocol,
- request smuggling or routing confusion that reaches an unintended handler,
- a panic or unbounded allocation reachable from an unauthenticated request,
- leaking internal details (stack traces, configuration, other users' data) in a
  response.

Out of scope: findings that require an insecure configuration the documentation
explicitly warns against, such as setting `AllowOrigins: []string{"*"}` together
with credentials, or running with `AllowAnonymous` enabled on a public MQTT port.

## Defaults worth knowing

- Panic recovery and security headers are installed by `goichi.New`.
- JWT signing is pinned to HMAC (HS256/384/512); `alg: none` and asymmetric-key
  confusion are rejected, and a valid `exp` claim is required.
- CORS allows no origin until you configure one.
- Handler errors are logged server-side and returned to the client as a generic
  message.
