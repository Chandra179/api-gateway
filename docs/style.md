# Go Style Guide

Baseline: [uber-go/guide](https://github.com/uber-go/guide). For anything
not covered below, that's the source of truth — go read the relevant
section there rather than asking "what's our style," and don't paste large
chunks of it back into this repo. This file only lists the rules that
either (a) directly explain a decision already made elsewhere in this repo,
or (b) come up often enough to be worth having locally.

See also: [`errors.md`](errors.md) for wrapping conventions,
[`logging.md`](logging.md) for logging conventions — both build on the
rules below.

---

## Errors

* **Handle errors once.** Don't `log.Error(err); return err` in the same
  function — pick one. This is *why* `RequestLog` is the single logging
  boundary for HTTP requests (see [`logging.md`](logging.md)): every layer
  below it wraps and returns, nothing below it logs.
* **Wrap with `%w`, obfuscate with `%v`.** Use `fmt.Errorf("...: %w", err)`
  when a caller up the chain might need `errors.Is`/`errors.As` on the root
  cause. Use `%v` when you deliberately don't want the internal error
  exposed for inspection (e.g. before it crosses a package boundary you
  don't control). See `errors.md` for the full layered pattern.
* **Naming**: exported sentinel errors are `ErrFoo`, unexported are
  `errFoo`, custom error types are `FooError`.
* **Don't panic** for anything recoverable — return an error. Panics are
  for truly irrecoverable states (or programmer bugs caught by `gin.CustomRecovery`
  at the HTTP boundary, which is a backstop, not a control-flow mechanism).

## Interfaces

* **Verify interface compliance at compile time**: `var _ Interface = (*Type)(nil)`
  next to the type definition, not discovered at runtime or in a test.
* **Accept interfaces as values, not pointers** — don't use pointer
  receivers for interface types.

## Program structure

* **Exit once, from `main()` only.** `os.Exit`/`log.Fatal` belong in
  `cmd/*/main.go` or the top-level `Run*` function (see
  `server/http_server.go`), never in a library/service/store function —
  those return errors and let the caller decide.
* **Avoid `init()`.** Prefer explicit constructors (`NewX(...)`) so
  initialization order is visible and testable.
* **Avoid mutable package-level globals.** Pass dependencies in explicitly
  (constructor injection) instead of reading/writing package vars.
* **Goroutines aren't fire-and-forget.** Anything spawned with `go` needs a
  defined stop condition the caller can wait on (`sync.WaitGroup`, a done
  channel) — no goroutines with no way to know when/if they finished.

## Naming & imports

* **Package names**: short, lowercase, one word, no underscores — a caller
  shouldn't need an import alias to avoid a collision with the package name
  itself.
* **Import order**: standard library, blank line, everything else. Enforced
  by `goimports` with `local-prefixes: brook` in `.golangci.yml` — third
  party and `brook/...` imports are separated too.
* **Struct literals use field names.** `Config{Level: "info"}`, not
  positional `Config{"info"}` — positional breaks silently when a field is
  added.

## What's already enforced by tooling

`golangci-lint` (`.golangci.yml`) already catches some of this mechanically
— `bodyclose`, `nilerr`, `nilnesserr`, `nilnil`, `govet` shadow/fieldalignment,
`gofmt`/`goimports`. Rules above are the ones that aren't (or can't be)
caught by a linter and rely on the reviewer/author knowing them.
