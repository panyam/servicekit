package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// OriginChecker rejects requests whose Origin header doesn't match
// an allowlist. Designed for WebSocket endpoints where CORS headers
// alone don't prevent cross-origin connections.
//
// Matching rules:
//   - Exact match on scheme+host (port-insensitive for 80/443)
//   - Wildcard subdomain: "*.example.com" matches "foo.example.com"
//   - "localhost" matches any localhost origin regardless of port
//   - Empty allowlist = allow all (no-op)
//   - Missing Origin header = blocked (unless allowlist is empty)
type OriginChecker struct {
	allowed []originPattern
}

type originPattern struct {
	raw       string
	host      string // "example.com"
	wildcard  bool   // true for "*.example.com"
	localhost bool   // true for "localhost"
}

// NewOriginChecker creates a checker from a list of allowed origins.
// Accepts formats: "https://example.com", "*.example.com", "localhost".
// Returns nil if the list is empty (caller should skip the check).
func NewOriginChecker(origins []string) *OriginChecker {
	if len(origins) == 0 {
		return nil
	}
	var patterns []originPattern
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		p := originPattern{raw: o}
		if o == "localhost" {
			p.localhost = true
		} else if strings.HasPrefix(o, "*.") {
			p.wildcard = true
			p.host = o[2:] // "*.example.com" → "example.com"
		} else {
			// Parse as URL to extract host
			if !strings.Contains(o, "://") {
				o = "https://" + o
			}
			if u, err := url.Parse(o); err == nil {
				p.host = u.Hostname()
			} else {
				p.host = o
			}
		}
		patterns = append(patterns, p)
	}
	if len(patterns) == 0 {
		return nil
	}
	return &OriginChecker{allowed: patterns}
}

// Check returns true if the origin is allowed.
func (c *OriginChecker) Check(origin string) bool {
	if c == nil {
		return true
	}
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()

	for _, p := range c.allowed {
		if p.localhost {
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				return true
			}
			continue
		}
		if p.wildcard {
			if host == p.host || strings.HasSuffix(host, "."+p.host) {
				return true
			}
			continue
		}
		if host == p.host {
			return true
		}
	}
	return false
}

// Middleware returns an HTTP middleware that rejects disallowed origins.
// Only applies to WebSocket upgrade requests (Connection: Upgrade).
// Non-upgrade requests pass through unchanged.
func (c *OriginChecker) Middleware(next http.Handler) http.Handler {
	if c == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only check WebSocket upgrades
		if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			origin := r.Header.Get("Origin")
			if !c.Check(origin) {
				slog.Warn("Rejected WebSocket origin", "component", "origin", "origin", origin, "ip", ClientIP(r), "path", r.URL.Path)
				http.Error(w, `{"error":"origin not allowed"}`, http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// NewLocalhostOriginChecker creates an OriginChecker that only allows
// localhost origins (localhost, 127.0.0.1, ::1). This is the secure default
// for local development servers that don't have an explicit allowlist.
//
// Unlike NewOriginChecker(nil) which returns nil (allow-all), this returns
// a restrictive checker that blocks remote origins.
func NewLocalhostOriginChecker() *OriginChecker {
	return NewOriginChecker([]string{"localhost"})
}

// CheckRequest validates an HTTP request's origin using both Origin and Host
// headers. This implements the full DNS rebinding protection pattern:
//
//   - Origin header present → delegates to Check(origin)
//   - Origin absent, Host is localhost → allow (local dev tools like curl)
//   - Origin absent, Host is remote → reject (potential DNS rebinding)
//   - No Origin or Host info → allow (same-origin or non-browser client)
//
// Nil-safe: calling on a nil *OriginChecker allows all requests.
func (c *OriginChecker) CheckRequest(r *http.Request) bool {
	if c == nil {
		return true
	}

	origin := r.Header.Get("Origin")
	if origin != "" {
		return c.Check(origin)
	}

	// No Origin header — fall back to Host header.
	host := r.Host
	if host == "" {
		host = r.Header.Get("Host")
	}
	if host == "" {
		return true // No origin info to validate
	}

	return isLocalhostHost(host)
}

// isLocalhostHost checks if a Host header value is a localhost variant.
// Strips the port if present before comparing.
func isLocalhostHost(host string) bool {
	h := host
	// Strip port. Handle IPv6 [::1]:port format.
	if idx := strings.LastIndex(h, ":"); idx != -1 {
		// Make sure we're not stripping part of an IPv6 address
		if bracketIdx := strings.LastIndex(h, "]"); bracketIdx < idx {
			h = h[:idx]
		}
	}
	// Strip brackets from IPv6
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}
