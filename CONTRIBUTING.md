# Contributing to Goichi

Thanks for taking the time to contribute.

## Getting started

```bash
git clone https://github.com/goichi-dev/goichi.git
cd goichi
go build ./...
```

Goichi requires Go 1.26 or later. The repository is a library: the only thing you
can run directly lives in
[goichi-examples](https://github.com/goichi-dev/goichi-examples), which is where
new runnable demos belong.

## Quality gate

Run all three before opening a pull request. CI runs the same checks.

```bash
gofmt -w .
go vet ./...
staticcheck ./...
```

## A green build is not a passing test

This matters more here than in most projects. A routing bug once survived
`gofmt`, `go vet` and `staticcheck` for its entire life because none of them
execute a request. **Before claiming a change to the router, middleware chain or
a protocol server works, start a server and send it real traffic.**

The [examples repository](https://github.com/goichi-dev/goichi-examples) is the
fastest way to do that: point it at your working tree with a `go.work` file and
exercise the protocol you touched.

## Tests

The repository intentionally ships without `_test.go` files. Writing a temporary
test to prove a fix is encouraged — please delete it before you open the pull
request, and describe what you observed in the PR body instead.

## Commit messages

Short, imperative, capitalised, no prefix:

```
Fix radix router param matching
Add automatic OPTIONS handling
```

## Pull requests

- Keep changes focused; unrelated refactors make review harder.
- Explain the behaviour before and after, not just the diff.
- Note any breaking change explicitly — the project is on `v0.x`, so breaking
  changes are allowed, but they must be called out for the changelog.
- New exported identifiers need a doc comment. They become the public reference
  on pkg.go.dev.

## Comments and language

Code, comments, documentation and commit messages are written in English so that
every contributor can read them.

## Reporting bugs

Open an issue with the Go version, the operating system, a minimal program that
reproduces the problem, and what you expected instead. For anything with security
impact, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.
