---
package: middleware
purpose: Reusable, application-agnostic HTTP middleware for production hardening (rate/conn limits, origin/CORS, recovery, logging, request IDs, body/content-type limits, health).
last_rebuilt: 04f5d50ab9801bba49c428990700c9aded8b119b
depends_on:
entities:
  - name: ClientIPExtractor
    kind: struct
    role: Extracts the real client IP, honoring X-Forwarded-For / X-Real-IP only from trusted proxy CIDRs.
    why: Spoofable forwarding headers are trusted only when RemoteAddr is in an allowlisted CIDR; an empty allowlist trusts everything for backwards-compat.
  - name: NewClientIPExtractor
    kind: func
    role: Builds an extractor from CIDR strings, auto-suffixing bare IPs with /32 or /128.
    why: Lets config accept plain "127.0.0.1" or "::1" without forcing users to write CIDR masks; malformed entries are silently dropped.
  - name: ClientIP
    kind: func
    role: Package-level convenience that extracts client IP via the shared default extractor.
    why: Used internally by logging/ratelimit/connlimit so all components see the same IP; configured globally via SetTrustedProxies.
  - name: SetTrustedProxies
    kind: func
    role: Reconfigures the package-level default extractor.
    why: Mutates global state — prefer NewClientIPExtractor for instance isolation; documented as a convenience-only path.
  - name: RateLimiter
    kind: struct
    role: Enforces a global token-bucket limit plus per-key buckets with TTL-based cleanup of stale keys.
    why: Per-key map is mutex-guarded and reaped by a background goroutine to prevent unbounded memory growth from one-shot keys; OnRejected hook decouples metrics from the limiter.
  - name: RateLimitConfig
    kind: struct
    role: Tunables for global rate, per-key rate/burst, and key-limiter TTL.
    why: Zero on both rate fields makes NewRateLimiter return nil (no-op), so config absence disables the feature cleanly.
  - name: NewRateLimiter
    kind: func
    role: Constructs a RateLimiter, returning nil when both limits are 0.
    why: Nil-return is the disable signal; only spawns the cleanup goroutine when KeyLimiterTTL > 0.
  - name: ConnLimiter
    kind: struct
    role: Caps concurrent in-flight requests using an atomic counter, rejecting overflow with 503.
    why: Aimed at WebSocket endpoints where a connection holds resources for its whole lifetime; counter decrements on handler return (defer), so the limit tracks live sessions not request rate.
  - name: NewConnLimiter
    kind: func
    role: Constructs a ConnLimiter, returning nil for max <= 0 (unlimited).
    why: Nil-return is the unlimited/disable signal consistent with other constructors.
  - name: OriginChecker
    kind: struct
    role: Allowlist matcher for Origin headers supporting exact, wildcard-subdomain, and localhost patterns.
    why: WebSocket upgrades bypass browser CORS enforcement, so origin must be checked server-side; "*" or empty list returns nil (allow-all), and nil is treated as allow-all everywhere.
  - name: NewOriginChecker
    kind: func
    role: Parses origin strings into match patterns; returns nil for empty or "*".
    why: Folds allow-all into the nil receiver so callers never special-case it.
  - name: NewLocalhostOriginChecker
    kind: func
    role: Builds a checker that only permits localhost/127.0.0.1/::1.
    why: Provides a secure-by-default for dev servers — distinct from NewOriginChecker(nil) which is allow-all.
  - name: OriginChecker.Check
    kind: func
    role: Returns whether a given Origin string is allowed.
    why: Nil receiver allows all; an empty origin string is rejected (a configured allowlist means cross-origin must be explicit).
  - name: OriginChecker.CheckRequest
    kind: func
    role: Full DNS-rebinding-protection check using Origin then falling back to the Host header.
    why: When Origin is absent it allows only localhost Hosts, blocking remote DNS-rebinding while still letting local CLI tools (curl) through.
  - name: OriginChecker.Middleware
    kind: func
    role: HTTP middleware that rejects disallowed origins on WebSocket upgrades only.
    why: Non-upgrade requests pass through untouched so the same checker can guard a mux that mixes WS and plain HTTP.
  - name: CORS
    kind: func
    role: Middleware that reflects allowed origins into CORS headers and answers OPTIONS preflight with 204.
    why: Reflects the specific origin (not "*") so credentialed requests work and to reuse the same OriginChecker as WS guarding; a mismatched origin gets no CORS headers so the browser blocks it.
  - name: CORSOption
    kind: type
    role: Functional option for customizing CORS methods/headers/exposed-headers.
    why: Keeps the CORS signature stable while allowing per-mount overrides of the GET/POST/OPTIONS + Content-Type/Authorization defaults.
  - name: RequireContentType
    kind: func
    role: Middleware rejecting POST/PUT/PATCH whose Content-Type doesn't prefix-match an accepted type (415).
    why: Defense-in-depth against CSRF — browsers can submit forms cross-origin without preflight, but only as form/text content types, never application/json.
  - name: BodyLimiter
    kind: struct
    role: Caps request body size via http.MaxBytesReader.
    why: Returns nil for maxBytes <= 0; enforcement is lazy (only triggers when the handler actually reads past the limit).
  - name: Recovery
    kind: func
    role: Middleware that recovers panics, logs stack trace, and returns 500.
    why: Without it a single panicking handler crashes the whole server process.
  - name: RequestLogger
    kind: func
    role: Middleware logging method/path/status/duration/IP via slog, with per-path skip list.
    why: Wraps ResponseWriter in statusRecorder to capture the status code; auto-includes request_id when present, and skips noisy paths like /healthz.
  - name: RequestID
    kind: struct
    role: Middleware that propagates an incoming X-Request-Id or generates a new 16-byte hex ID.
    why: Stores the ID in context (private key type) and echoes it on the response so logs and downstream calls share one correlation ID.
  - name: RequestIDFromContext
    kind: func
    role: Reads the request ID out of a context.
    why: The context key is unexported, so this is the only retrieval path; returns "" when absent (used by RequestLogger).
  - name: ContextWithRequestID
    kind: func
    role: Stores a request ID into a context.
    why: Exported companion to the unexported key, letting callers seed an ID outside the middleware.
  - name: HealthCheck
    kind: struct
    role: http.Handler serving a JSON health/readiness endpoint with optional readiness callback.
    why: Mounted directly on a mux (not chained as middleware); nil receiver returns 404 and a false readyFunc returns 503 for k8s/LB probes.
  - name: HealthOption
    kind: type
    role: Functional option configuring HealthCheck path and readiness func.
    why: WithPath/WithReadyFunc keep construction declarative; default path is /healthz.
  - name: Guard
    kind: struct
    role: Composes a chain of middleware into a single Wrap call.
    why: Use-order is outermost-first but applied in reverse internally; nil middleware are skipped on Use and a nil Guard passes the handler through.
  - name: ApplyDefaults
    kind: func
    role: Sets read/write/idle/header timeout defaults on an http.Server, only filling zero fields.
    why: Never overwrites caller-set values, so SSE endpoints can pre-set (or later zero) WriteTimeout to avoid the server killing long-lived streams.
---

Design notes:

- Nil-as-disable is the package's central convention. Constructors (NewBodyLimiter, NewConnLimiter, NewRateLimiter, NewOriginChecker) return nil when the feature should be off, and every method/Middleware has a nil-receiver guard that passes the request through. This lets a server wire all middleware unconditionally and disable individual ones purely via config.

- IP attribution is centralized: ClientIP / the package-level defaultExtractor is the single source of truth, consumed by logging, ratelimit, and connlimit. Configure it once via SetTrustedProxies (or use per-instance ClientIPExtractor) so spoofed X-Forwarded-For headers are only honored behind trusted proxies.

- The OriginChecker is deliberately reused for two distinct jobs: server-side WebSocket upgrade gating (where browser CORS does not apply) and CORS header reflection. Allow-all collapses to a nil checker in both cases.

- Lifecycle/ordering gotchas: Guard applies middleware in reverse so the first Use'd entry is outermost (runs first on request, last on response). RequestID must run before RequestLogger for the request_id field to appear in logs. ConnLimiter decrements on handler return, so for WebSocket handlers the slot is held for the whole session — sizing must reflect concurrent connections, not request throughput. RateLimiter's per-key map is reaped by a background goroutine only when KeyLimiterTTL > 0.
