package ratelimit

import (
	"errors"
	"strconv"
)

// checkScript atomically evaluates both the token bucket and the daily quota
// for one client in a single EVAL. Keys:
//
//	KEYS[1] = token bucket key (hash: tokens, ls=last refill unix second)
//	KEYS[2] = daily quota key  (string counter with 24h TTL)
//
// Args:
//
//	ARGV[1] = burst refill rate (tokens/second)
//	ARGV[2] = bucket capacity / burst size
//	ARGV[3] = daily quota limit
//	ARGV[4] = current unix time (seconds)
//	ARGV[5] = quota window length (seconds)
//
// Returns {allowed, reason, tokens, quotaCount, retryAfter}.
//
// The token bucket refills continuously from elapsed time (no fixed-window
// boundary burst, per the design doc) and the daily quota uses a plain INCR so
// it is cheap and durable enough for a 24h horizon.
const checkScript = `
local bucket = redis.call('HMGET', KEYS[1], 'tokens', 'ls')
local now = tonumber(ARGV[4])

local tokens
if bucket[1] == false then
  -- First request for this client: start with a full bucket.
  tokens = tonumber(ARGV[2])
  redis.call('HMSET', KEYS[1], 'tokens', tokens, 'ls', now)
else
  -- Refill continuously based on elapsed time.
  local last = tonumber(bucket[2])
  if last == nil then last = now end
  tokens = tonumber(bucket[1]) + (now - last) * tonumber(ARGV[1])
  local cap = tonumber(ARGV[2])
  if tokens > cap then tokens = cap end
end

local allowed = 1
local reason = 'ok'
local retryAfter = 0
local quotaCount = 0

if tokens < 1 then
  -- Burst exhausted: reject and compute how long until one token refills.
  allowed = 0
  reason = 'burst'
  retryAfter = math.ceil((1 - tokens) / tonumber(ARGV[1]))
  if retryAfter < 0 then retryAfter = 0 end
else
  -- Take one token, then count toward the daily quota (fixed window).
  tokens = tokens - 1
  redis.call('HMSET', KEYS[1], 'tokens', tokens, 'ls', now)
  quotaCount = redis.call('INCR', KEYS[2])
  if quotaCount == 1 then
    redis.call('EXPIRE', KEYS[2], tonumber(ARGV[5]))
  end
  if quotaCount > tonumber(ARGV[3]) then
    allowed = 0
    reason = 'quota'
  end
end

return {allowed, reason, tokens, quotaCount, retryAfter}
`

var (
	errStoreNil        = errors.New("rate-limit key missing")
	errBadScriptResult = errors.New("rate-limit script returned unexpected shape")
)

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	default:
		return 0
	}
}

func toFloat64(v interface{}) float64 {
	switch t := v.(type) {
	case int64:
		return float64(t)
	case int:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	default:
		return 0
	}
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
