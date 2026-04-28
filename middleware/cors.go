package middleware

import (
	"net/http"
	"strings"
)

// CORSOption configures the CORS middleware.
type CORSOption func(*corsConfig)

type corsConfig struct {
	methods       string
	headers       string
	exposeHeaders string
}

func defaultCORSConfig() corsConfig {
	return corsConfig{
		methods: "GET, POST, OPTIONS",
		headers: "Content-Type, Authorization",
	}
}

// CORSAllowMethods overrides the default allowed methods (GET, POST, OPTIONS).
func CORSAllowMethods(methods ...string) CORSOption {
	return func(c *corsConfig) { c.methods = strings.Join(methods, ", ") }
}

// CORSAllowHeaders overrides the default allowed headers (Content-Type, Authorization).
func CORSAllowHeaders(headers ...string) CORSOption {
	return func(c *corsConfig) { c.headers = strings.Join(headers, ", ") }
}

// CORSExposeHeaders sets the Access-Control-Expose-Headers value.
// By default no headers are exposed.
func CORSExposeHeaders(headers ...string) CORSOption {
	return func(c *corsConfig) { c.exposeHeaders = strings.Join(headers, ", ") }
}

// CORS returns middleware that sets CORS headers based on an OriginChecker.
//
// Behavior:
//   - If checker is nil (no allowlist): reflect any origin back
//   - If checker is set and the request Origin matches: reflect the origin back
//   - If checker is set and the request Origin doesn't match: no CORS headers
//     (browser will block the response)
//   - OPTIONS preflight requests are handled and return 204
//
// Use CORSOption values to customize allowed methods, headers, and exposed headers.
// Defaults: methods GET/POST/OPTIONS, headers Content-Type/Authorization, no exposed headers.
//
// This replaces the naive "Access-Control-Allow-Origin: *" pattern with
// origin validation. The same OriginChecker used for WebSocket origin checks
// is reused here for consistency.
func CORS(checker *OriginChecker, opts ...CORSOption) func(http.Handler) http.Handler {
	cfg := defaultCORSConfig()
	for _, o := range opts {
		o(&cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" {
				if checker == nil || checker.Check(origin) {
					// Reflect the specific origin (required for credentials, more secure than *)
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
				}
				// If origin doesn't match, we don't set any CORS headers —
				// the browser will block the response.
			}

			w.Header().Set("Access-Control-Allow-Methods", cfg.methods)
			w.Header().Set("Access-Control-Allow-Headers", cfg.headers)
			if cfg.exposeHeaders != "" {
				w.Header().Set("Access-Control-Expose-Headers", cfg.exposeHeaders)
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
