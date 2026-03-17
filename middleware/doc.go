// Package middleware provides reusable HTTP middleware for production hardening.
//
// Components include:
//   - ClientIPExtractor: Extracts real client IPs behind trusted reverse proxies
//   - RateLimiter: Global and per-key rate limiting with configurable key functions
//   - ConnLimiter: Concurrent connection limiting for WebSocket endpoints
//   - OriginChecker: Origin allowlist for WebSocket upgrade requests
//   - CORS: Origin-aware CORS header middleware
//   - Recovery: Panic recovery with structured logging
//   - RequestLogger: Structured HTTP request logging
//   - Guard: Composable middleware chain
//
// All components are nil-safe: a nil component acts as a no-op passthrough.
// No application-specific imports — designed for embedding in any Go HTTP server.
package middleware
