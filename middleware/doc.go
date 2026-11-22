// Package middleware provides reusable HTTP middleware for production hardening.
//
// Components include:
//   - TrustedProxies: Client IP and HTTPS answers behind a proxy that ends TLS;
//     forwarding headers count only from listed proxies, and an empty list trusts nobody
//   - ClientIPExtractor: Older client-IP wrapper over TrustedProxies
//   - RateLimiter: Global and per-key rate limiting with configurable key functions
//   - ConnLimiter: Concurrent connection limiting for WebSocket endpoints
//   - OriginChecker: Origin allowlist for WebSocket upgrade requests
//   - CORS: Origin-aware CORS header middleware
//   - Recovery: Panic recovery with structured logging
//   - RequestLogger: Structured HTTP request logging (includes request ID when available)
//   - RequestID: Request ID generation/propagation via X-Request-Id header
//   - BodyLimiter: Request body size limiting via http.MaxBytesReader
//   - HealthCheck: Health/readiness endpoint handler for load balancers and k8s probes
//   - Guard: Composable middleware chain
//
// Helpers:
//   - CheckPlaintext: Startup check that refuses credentials over plain HTTP
//     on a non-loopback address
//   - ApplyDefaults: Sets sensible timeout defaults on http.Server
//     (note: SSE endpoints require WriteTimeout = 0 separately to prevent
//     the server from closing long-lived streaming connections)
//
// All components are nil-safe: a nil component acts as a no-op passthrough.
// No application-specific imports — designed for embedding in any Go HTTP server.
package middleware
