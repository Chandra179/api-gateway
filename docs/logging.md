# Logging Standards

How we log: which level to use, what a log line contains, what never
belongs in one. Builds on [`errors.md`](errors.md) — that covers wrapping
errors on the way up; this covers what happens once one reaches a logger.

We use [`go.uber.org/zap`](https://pkg.go.dev/go.uber.org/zap), configured
in `logger/logger.go`.

---

## Golden Rules

* **One line per request.** All request-scoped logging happens in
  `RequestLog` (`middleware/request_log.go`), not scattered through
  handlers — the canonical-log-line pattern. Don't add extra
  `logger.Info/Warn/Error(...)` calls in a handler for something the
  request line already covers; that's a duplicate entry for one event.
* **Level tracks response status, not developer judgment.** `RequestLog`
  derives Info (2xx/3xx) / Warn (4xx) / Error (5xx) from the status code.
  Don't hand-pick a level by feel.
* **Never log request or response bodies, at any status.** After decoding,
  the body isn't what the client sent anyway (whitespace/key-order/unknown
  fields lost), so there's no debugging value — and 4xx bodies are exactly
  where secrets are most likely to appear (a failed login's password), so
  gating on status makes it worse, not safer. Use the request ID to
  correlate instead.
* **Never log secrets** — auth headers, tokens, passwords, API keys — none
  of it, at any level.

## Levels

| Level | Meaning | Example |
|---|---|---|
| `Debug` | Internal state, off in production. | "cache miss for key X" |
| `Info` | Normal operation. | request completed, 2xx/3xx |
| `Warn` | Client-caused, expected. Not paged. | request completed, 4xx |
| `Error` | Server-caused, unexpected. Gets looked at. | request completed, 5xx; unhandled panic |

## Surfacing an error from a handler

Call `c.Error(err)` before writing the response; `RequestLog` attaches the
last recorded error to the line and logs it. You don't call the logger
yourself:

```go
if err != nil {
	_ = c.Error(fmt.Errorf("get example %s: %w", id, err)) // wrap per errors.md
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	return
}
```

## Stack traces

zap attaches these automatically based on level — never add one by hand
(`debug.Stack()` in a handler, etc.):

* **Production**: attached at `Error`+ only (5xx, panics). Nothing in a
  4xx's stack that isn't already in the error message.
* **Development**: attached at `Warn`+ (4xx too) — useful locally, too
  noisy for production volume.

Caveat: the stack is captured **where the log call happens** (inside
`RequestLog`), not where the error originated. For a panic recovered by
`gin.CustomRecovery` that's exactly right. For an ordinary wrapped error
bubbling up from a store/service call, it's the middleware's stack, not the
failure site — if you need the real origin, that's a future trace span's
job, not this log line.

## Logger setup

`logger.NewLogger(appEnvironment)` picks the zap preset from
`APP_ENVIRONMENT`: `dev` → `zap.NewDevelopment()` (console, Debug+,
stacktrace from Warn), anything else → `zap.NewProduction()` (JSON, Info+,
stacktrace from Error). One environment-wide switch, no per-package
override today.
