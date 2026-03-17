package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP_DirectConnection(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "203.0.113.50:12345"

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %q", got)
	}
}

func TestClientIP_XForwardedFor_NoTrustedProxies(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %q", got)
	}
}

func TestClientIP_XForwardedFor_MultipleIPs(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 198.51.100.1, 10.0.0.1")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected leftmost IP 203.0.113.50, got %q", got)
	}
}

func TestClientIP_XForwardedFor_TrustedProxy(t *testing.T) {
	SetTrustedProxies([]string{"127.0.0.1", "::1"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50 from trusted proxy, got %q", got)
	}
}

func TestClientIP_XForwardedFor_UntrustedProxy(t *testing.T) {
	SetTrustedProxies([]string{"127.0.0.1"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "198.51.100.99:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	got := ClientIP(req)
	if got != "198.51.100.99" {
		t.Errorf("expected direct IP 198.51.100.99 (untrusted proxy), got %q", got)
	}
}

func TestClientIP_XRealIP_TrustedProxy(t *testing.T) {
	SetTrustedProxies([]string{"10.0.0.0/8"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.50")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50 from X-Real-IP, got %q", got)
	}
}

func TestClientIP_XRealIP_UntrustedProxy(t *testing.T) {
	SetTrustedProxies([]string{"127.0.0.1"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "198.51.100.99:12345"
	req.Header.Set("X-Real-IP", "10.0.0.1")

	got := ClientIP(req)
	if got != "198.51.100.99" {
		t.Errorf("expected direct IP (untrusted proxy), got %q", got)
	}
}

func TestClientIP_XForwardedFor_TakesPrecedenceOverXRealIP(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.Header.Set("X-Real-IP", "198.51.100.1")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected X-Forwarded-For to take precedence, got %q", got)
	}
}

func TestClientIP_IPv6_RemoteAddr(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[2001:db8::1]:12345"

	got := ClientIP(req)
	if got != "2001:db8::1" {
		t.Errorf("expected 2001:db8::1, got %q", got)
	}
}

func TestClientIP_IPv6_Localhost_TrustedProxy(t *testing.T) {
	SetTrustedProxies([]string{"::1"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "[::1]:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %q", got)
	}
}

func TestClientIP_DockerBridgeNetwork(t *testing.T) {
	SetTrustedProxies([]string{"172.17.0.0/16"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "172.17.0.2:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected client IP from Docker proxy, got %q", got)
	}
}

func TestClientIP_DockerBridge_OutsideRange(t *testing.T) {
	SetTrustedProxies([]string{"172.17.0.0/16"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "172.18.0.5:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")

	got := ClientIP(req)
	if got != "172.18.0.5" {
		t.Errorf("expected direct IP (proxy not in trusted range), got %q", got)
	}
}

func TestClientIP_MultipleTrustedCIDRs(t *testing.T) {
	SetTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8", "172.17.0.0/16"})

	tests := []struct {
		remoteAddr string
		xff        string
		expected   string
	}{
		{"127.0.0.1:1234", "203.0.113.1", "203.0.113.1"},
		{"10.0.0.5:1234", "203.0.113.2", "203.0.113.2"},
		{"172.17.0.3:1234", "203.0.113.3", "203.0.113.3"},
		{"192.168.1.1:1234", "spoofed", "192.168.1.1"}, // not trusted
	}

	for _, tc := range tests {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = tc.remoteAddr
		req.Header.Set("X-Forwarded-For", tc.xff)

		got := ClientIP(req)
		if got != tc.expected {
			t.Errorf("remoteAddr=%s xff=%s: expected %q, got %q",
				tc.remoteAddr, tc.xff, tc.expected, got)
		}
	}
}

func TestClientIP_WhitespaceInXFF(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "  203.0.113.50 , 198.51.100.1 ")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected trimmed IP, got %q", got)
	}
}

func TestClientIP_EmptyXFF(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "203.0.113.50:12345"
	req.Header.Set("X-Forwarded-For", "")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected direct IP for empty XFF, got %q", got)
	}
}

func TestClientIP_BareIPWithoutPort(t *testing.T) {
	SetTrustedProxies(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "203.0.113.50"

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %q", got)
	}
}

func TestClientIP_RateLimitBypass(t *testing.T) {
	SetTrustedProxies([]string{"127.0.0.1"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "198.51.100.99:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	got := ClientIP(req)
	if got != "198.51.100.99" {
		t.Fatalf("SECURITY: attacker spoofed IP to %q, should be 198.51.100.99", got)
	}
}

func TestSetTrustedProxies_BareIPs(t *testing.T) {
	SetTrustedProxies([]string{"10.0.0.1", "::1"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	if got := ClientIP(req); got != "203.0.113.50" {
		t.Errorf("bare IPv4: expected 203.0.113.50, got %q", got)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "[::1]:12345"
	req2.Header.Set("X-Forwarded-For", "203.0.113.60")
	if got := ClientIP(req2); got != "203.0.113.60" {
		t.Errorf("bare IPv6: expected 203.0.113.60, got %q", got)
	}
}

func TestSetTrustedProxies_InvalidCIDR(t *testing.T) {
	SetTrustedProxies([]string{"not-a-cidr", "127.0.0.1"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	got := ClientIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected valid CIDR to still work, got %q", got)
	}
}

// TestClientIPExtractor_Instance verifies that two extractors with different
// trusted CIDRs do not interfere with each other.
func TestClientIPExtractor_Instance(t *testing.T) {
	e1 := NewClientIPExtractor([]string{"127.0.0.1"})
	e2 := NewClientIPExtractor([]string{"10.0.0.0/8"})

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	// e1 does not trust 10.0.0.1 → should return direct IP
	if got := e1.ExtractClientIP(req); got != "10.0.0.1" {
		t.Errorf("e1: expected direct IP 10.0.0.1, got %q", got)
	}

	// e2 trusts 10.0.0.0/8 → should return XFF IP
	if got := e2.ExtractClientIP(req); got != "203.0.113.50" {
		t.Errorf("e2: expected XFF IP 203.0.113.50, got %q", got)
	}
}

// TestClientIPExtractor_NilCIDRsTrustsAll verifies default trust-all behavior.
func TestClientIPExtractor_NilCIDRsTrustsAll(t *testing.T) {
	e := NewClientIPExtractor(nil)

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	if got := e.ExtractClientIP(req); got != "203.0.113.50" {
		t.Errorf("expected XFF trusted, got %q", got)
	}
}

func TestClientIP_IntegrationWithRateLimit(t *testing.T) {
	SetTrustedProxies([]string{"10.0.0.1"})

	rl := NewRateLimiter(RateLimitConfig{
		PerKeyPerSec: 1,
		PerKeyBurst:  1,
	})

	handler := rl.Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from real client IP (via trusted proxy) — should pass
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "10.0.0.1:12345"
	req1.Header.Set("X-Forwarded-For", "203.0.113.50")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("first request should pass, got %d", rr1.Code)
	}

	// Second request from same real IP — should be rate limited
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	req2.Header.Set("X-Forwarded-For", "203.0.113.50")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be rate limited, got %d", rr2.Code)
	}

	// Third request with DIFFERENT real IP — should pass
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "10.0.0.1:12345"
	req3.Header.Set("X-Forwarded-For", "203.0.113.51")
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("different IP should pass, got %d", rr3.Code)
	}
}

func TestClientIP_IntegrationSpoofAttempt(t *testing.T) {
	SetTrustedProxies([]string{"127.0.0.1"})

	rl := NewRateLimiter(RateLimitConfig{
		PerKeyPerSec: 1,
		PerKeyBurst:  1,
	})

	handler := rl.Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "198.51.100.99:12345"
	req1.Header.Set("X-Forwarded-For", "1.1.1.1")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("first request should pass, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "198.51.100.99:12345"
	req2.Header.Set("X-Forwarded-For", "2.2.2.2")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("SECURITY: attacker bypassed rate limit by spoofing XFF (got %d)", rr2.Code)
	}
}
