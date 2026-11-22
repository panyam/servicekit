package middleware

import (
	"net/http"
	"net/netip"
)

// ClientIPExtractor extracts the real client IP from requests. It wraps a
// TrustedProxies and answers with TrustedProxies.ClientIP.
//
// Usage:
//
//	// Trust Caddy on localhost and the Docker bridge network
//	extractor := NewClientIPExtractor([]string{"127.0.0.1/32", "172.17.0.0/16", "::1/128"})
//	ip := extractor.ExtractClientIP(r)
//
// New code that makes security decisions should use TrustedProxies directly,
// where an empty list trusts nobody and bad entries are errors.
type ClientIPExtractor struct {
	proxies TrustedProxies
}

// trustEveryone is the list NewClientIPExtractor uses for an empty input.
var trustEveryone = TrustedProxies{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")}

// defaultExtractor is used by the package-level ClientIP convenience function.
var defaultExtractor = &ClientIPExtractor{proxies: trustEveryone}

// NewClientIPExtractor creates an extractor with the given trusted proxy
// CIDRs or bare addresses. Entries that fail to parse are dropped.
//
// Deprecated behavior: when cidrs is nil, empty, or holds no valid entry,
// every peer is trusted, so any client can set its own X-Forwarded-For.
// That default is kept for compatibility and will change to trust nobody in
// a later release. Use ParseTrustedProxies, which trusts nobody when empty
// and reports bad entries, and SetDefaultTrustedProxies for the
// package-level ClientIP. See UPGRADING.md.
func NewClientIPExtractor(cidrs []string) *ClientIPExtractor {
	var tp TrustedProxies
	for _, c := range cidrs {
		if p, err := ParseTrustedProxies([]string{c}); err == nil {
			tp = append(tp, p...)
		}
	}
	if len(tp) == 0 {
		tp = trustEveryone
	}
	return &ClientIPExtractor{proxies: tp}
}

// SetTrustedProxies configures the default (package-level) extractor with
// NewClientIPExtractor's rules, including its deprecated trust-everyone
// default for an empty list. Prefer SetDefaultTrustedProxies.
//
// Like SetDefaultTrustedProxies, it is not safe to call concurrently with
// requests. Call it once at startup.
func SetTrustedProxies(cidrs []string) {
	defaultExtractor = NewClientIPExtractor(cidrs)
}

// SetDefaultTrustedProxies configures the package-level ClientIP, which
// RequestLogger, ConnLimiter, Recovery, OriginChecker and RateLimiter (with
// a nil keyFunc) use. An empty tp trusts nobody, unlike SetTrustedProxies.
//
// It is not safe to call concurrently with requests. Call it once at
// startup.
func SetDefaultTrustedProxies(tp TrustedProxies) {
	defaultExtractor = &ClientIPExtractor{proxies: tp}
}

// ClientIP extracts the real client IP using the default extractor.
// Configure trusted proxies via SetDefaultTrustedProxies. Until that is
// called, every peer is trusted (see NewClientIPExtractor).
func ClientIP(r *http.Request) string {
	return defaultExtractor.ExtractClientIP(r)
}

// ExtractClientIP returns TrustedProxies.ClientIP for the extractor's list.
// X-Forwarded-For and X-Real-IP count only when RemoteAddr is a trusted
// proxy, and X-Forwarded-For is walked right to left, skipping trusted hops.
// With the trust-everyone default every hop is trusted, so the leftmost
// X-Forwarded-For entry wins.
func (e *ClientIPExtractor) ExtractClientIP(r *http.Request) string {
	return e.proxies.ClientIP(r)
}
