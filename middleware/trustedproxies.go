package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TrustedProxies lists the reverse proxies and load balancers in front of
// this process. Forwarding headers (X-Forwarded-For, X-Real-IP,
// X-Forwarded-Proto, Forwarded) are client-supplied, so they count only when
// the TCP peer is in this list (CWE-348).
//
// An empty or nil list trusts nobody. Every request then reports its TCP
// peer as the client, and IsHTTPS reports only whether this process ended
// TLS itself. To trust every peer, list 0.0.0.0/0 and ::/0, and only do that
// when the network guarantees nothing but the proxy can reach this port.
//
// The zero value is ready to use. A TrustedProxies is read-only after
// parsing and safe for concurrent use.
type TrustedProxies []netip.Prefix

// ParseTrustedProxies parses CIDRs ("10.0.0.0/8") and single addresses
// ("192.0.2.10", "::1") into TrustedProxies. Blank entries are skipped. Any
// other entry that fails to parse returns an error naming it, so a typo in
// operator config fails at startup instead of trusting less than intended.
func ParseTrustedProxies(entries []string) (TrustedProxies, error) {
	var out TrustedProxies
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.Contains(e, "/") {
			p, err := netip.ParsePrefix(e)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q: %w", e, err)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(e)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", e, err)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// Contains reports whether addr is one of the trusted proxies. An
// IPv4-mapped IPv6 address ("::ffff:10.0.0.1") matches its IPv4 form.
func (tp TrustedProxies) Contains(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, p := range tp {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIP returns the address a request came from, for rate limiting and
// logging. Its signature fits RateLimiter.Middleware's keyFunc, so
// rl.Middleware(tp.ClientIP) keys buckets by it.
//
// When the TCP peer is not trusted, the answer is the peer, whatever the
// headers say. Otherwise anyone could send a fresh X-Forwarded-For with
// every request and get a fresh rate-limit bucket each time.
//
// When the peer is trusted, X-Forwarded-For is walked right to left. Each
// trusted hop vouches for the entry to its left, so the first untrusted
// entry is the client. Entries to its left were written by the client and
// are ignored. When every entry is trusted, the leftmost one is the client.
// An entry that does not parse as an address stops the walk and returns the
// peer. X-Real-IP is read only when a trusted peer sent no X-Forwarded-For.
func (tp TrustedProxies) ClientIP(r *http.Request) string {
	peerStr, peer, ok := tp.trustedPeer(r)
	if !ok {
		return peerStr
	}
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		hops := strings.Split(strings.Join(xff, ","), ",")
		var last netip.Addr
		for i := len(hops) - 1; i >= 0; i-- {
			s := strings.TrimSpace(hops[i])
			if s == "" {
				continue
			}
			hop, err := netip.ParseAddr(s)
			if err != nil {
				return peer.String()
			}
			hop = hop.Unmap()
			if !tp.Contains(hop) {
				return hop.String()
			}
			last = hop
		}
		if last.IsValid() {
			return last.String()
		}
	}
	if xri, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
		return xri.Unmap().String()
	}
	return peer.String()
}

// IsHTTPS reports whether the client reached this server over HTTPS, for
// decisions like marking a cookie Secure. It is true when this process ended
// TLS itself (r.TLS is set). Behind a proxy that ends TLS the request arrives
// as plain HTTP, so it is also true when the TCP peer is trusted and the
// proxy says https. The proxy says so in the first X-Forwarded-Proto value,
// or, when that header is absent, in the proto of the first Forwarded
// element (RFC 7239). Only the first value counts, because the proxy nearest
// the client writes it.
//
// From an untrusted peer both headers are ignored and the answer is
// r.TLS != nil. An empty list therefore never reads them.
func (tp TrustedProxies) IsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if _, _, ok := tp.trustedPeer(r); !ok {
		return false
	}
	if xfp := r.Header.Values("X-Forwarded-Proto"); len(xfp) > 0 {
		first, _, _ := strings.Cut(xfp[0], ",")
		return strings.EqualFold(strings.TrimSpace(first), "https")
	}
	if fwd := r.Header.Values("Forwarded"); len(fwd) > 0 {
		first, _, _ := strings.Cut(fwd[0], ",")
		for _, pair := range strings.Split(first, ";") {
			k, v, _ := strings.Cut(strings.TrimSpace(pair), "=")
			if strings.EqualFold(strings.TrimSpace(k), "proto") {
				return strings.EqualFold(strings.Trim(strings.TrimSpace(v), `"`), "https")
			}
		}
	}
	return false
}

// trustedPeer parses r.RemoteAddr and reports whether it is in tp. It
// returns the peer's host string either way, for callers that fall back to
// it when the peer does not parse.
func (tp TrustedProxies) trustedPeer(r *http.Request) (string, netip.Addr, bool) {
	peerStr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peerStr); err == nil {
		peerStr = host
	}
	peer, err := netip.ParseAddr(peerStr)
	if err != nil {
		return peerStr, peer, false
	}
	peer = peer.Unmap()
	return peer.String(), peer, tp.Contains(peer)
}

// ErrPlaintextCredentials is returned by CheckPlaintext when a service would
// accept credentials over plain HTTP from the network.
var ErrPlaintextCredentials = errors.New("servicekit/middleware: refusing to serve credentials over plain HTTP on a non-loopback address; enable TLS or name the trusted proxies that end it")

// CheckPlaintext is a startup check for a service that accepts credentials
// (passwords, bearer tokens, session cookies). Call it with the listen
// address, whether this process serves TLS, and the trusted proxies.
//
// It returns nil when any of these holds:
//   - tls is true;
//   - proxies is non-empty, so a proxy is expected to end TLS in front;
//   - addr binds only a loopback address ("127.0.0.1:8080", "[::1]:8080",
//     "localhost:8080").
//
// Otherwise it returns an error wrapping ErrPlaintextCredentials. That
// includes ":8080" and "0.0.0.0:8080", which bind every interface, and any
// hostname other than "localhost", since a name can resolve anywhere. An
// addr that is not host:port returns a parse error instead.
//
// Naming proxies does not prove TLS is on in front of them. The check
// catches the default-config mistake, where a service meant for a proxy is
// started bare on a public port.
func CheckPlaintext(addr string, tls bool, proxies TrustedProxies) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("servicekit/middleware: listen address %q: %w", addr, err)
	}
	if tls || len(proxies) > 0 || isLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf("%w (addr %q)", ErrPlaintextCredentials, addr)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && a.Unmap().IsLoopback()
}
