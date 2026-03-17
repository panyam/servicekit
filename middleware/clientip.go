package middleware

import (
	"net"
	"net/http"
	"strings"
)

// ClientIPExtractor extracts the real client IP from requests, honoring
// X-Forwarded-For and X-Real-IP headers only when the direct connection
// comes from a trusted proxy CIDR.
//
// Usage:
//
//	// Trust Caddy on localhost and Docker bridge network
//	extractor := NewClientIPExtractor([]string{"127.0.0.1/32", "172.17.0.0/16", "::1/128"})
//	ip := extractor.ClientIP(r)
//
//	// Trust all proxies (suitable for single-proxy deployments)
//	extractor := NewClientIPExtractor(nil)
type ClientIPExtractor struct {
	trustedCIDRs []*net.IPNet
}

// defaultExtractor is used by the package-level ClientIP convenience function.
var defaultExtractor = &ClientIPExtractor{}

// NewClientIPExtractor creates an extractor with the given trusted proxy CIDRs.
// When cidrs is nil/empty, all proxies are trusted (backwards-compatible default).
func NewClientIPExtractor(cidrs []string) *ClientIPExtractor {
	e := &ClientIPExtractor{}
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		// Support bare IPs like "127.0.0.1" by appending /32 or /128
		if !strings.Contains(cidr, "/") {
			if strings.Contains(cidr, ":") {
				cidr += "/128"
			} else {
				cidr += "/32"
			}
		}
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			e.trustedCIDRs = append(e.trustedCIDRs, network)
		}
	}
	return e
}

// SetTrustedProxies configures the default (package-level) extractor.
// This is a convenience for simple use cases; prefer NewClientIPExtractor
// for instance isolation.
func SetTrustedProxies(cidrs []string) {
	defaultExtractor = NewClientIPExtractor(cidrs)
}

// ClientIP extracts the real client IP using the default extractor.
// Configure trusted proxies via SetTrustedProxies.
func ClientIP(r *http.Request) string {
	return defaultExtractor.ExtractClientIP(r)
}

// ExtractClientIP extracts the real client IP from the request.
//
// If trusted proxies are configured, X-Forwarded-For is only honored when
// the direct connection (RemoteAddr) comes from a trusted proxy CIDR.
// Otherwise, the direct RemoteAddr is used.
//
// If no trusted proxies are configured (default), X-Forwarded-For is
// always trusted (backwards-compatible for deployments behind a proxy).
func (e *ClientIPExtractor) ExtractClientIP(r *http.Request) string {
	directIP := extractIP(r.RemoteAddr)

	// Check X-Forwarded-For only if we trust the direct connection
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if e.isTrustedProxy(directIP) {
			// Use the leftmost (client-originated) IP
			if i := strings.IndexByte(xff, ','); i > 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}

	// Check X-Real-IP as fallback (single-hop proxies like nginx/Caddy)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if e.isTrustedProxy(directIP) {
			return strings.TrimSpace(xri)
		}
	}

	return directIP
}

// isTrustedProxy checks if the given IP is from a trusted proxy.
// If no trusted proxies are configured, all are trusted (backwards-compat).
func (e *ClientIPExtractor) isTrustedProxy(ip string) bool {
	if len(e.trustedCIDRs) == 0 {
		return true // no restrictions configured
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range e.trustedCIDRs {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}

// extractIP extracts the IP address from a RemoteAddr string (host:port or [host]:port).
func extractIP(addr string) string {
	// Handle IPv6 [::1]:port
	if strings.HasPrefix(addr, "[") {
		if i := strings.LastIndex(addr, "]"); i >= 0 {
			return addr[1:i]
		}
	}
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		return addr[:i]
	}
	return addr
}
