package middleware

import (
	"net/http/httptest"
	"testing"
)

// TestOriginChecker_CheckRequest_OriginPresent verifies that CheckRequest
// delegates to Check when the Origin header is present. The result should
// match the allowlist regardless of the Host header.
func TestOriginChecker_CheckRequest_OriginPresent(t *testing.T) {
	c := NewOriginChecker([]string{"example.com"})
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Origin", "https://example.com")

	if !c.CheckRequest(r) {
		t.Error("CheckRequest should allow matching origin")
	}

	r2 := httptest.NewRequest("POST", "/mcp", nil)
	r2.Header.Set("Origin", "https://evil.com")

	if c.CheckRequest(r2) {
		t.Error("CheckRequest should reject non-matching origin")
	}
}

// TestOriginChecker_CheckRequest_NoOrigin_LocalhostHost verifies that when
// no Origin header is present, CheckRequest falls back to the Host header
// and allows localhost variants. This prevents DNS rebinding attacks while
// supporting local development without Origin headers (e.g., curl, Postman).
func TestOriginChecker_CheckRequest_NoOrigin_LocalhostHost(t *testing.T) {
	c := NewOriginChecker([]string{"example.com"})

	for _, host := range []string{"localhost:8080", "127.0.0.1:3000", "[::1]:9090"} {
		r := httptest.NewRequest("POST", "/mcp", nil)
		r.Host = host
		if !c.CheckRequest(r) {
			t.Errorf("CheckRequest should allow localhost Host %q when Origin absent", host)
		}
	}
}

// TestOriginChecker_CheckRequest_NoOrigin_RemoteHost verifies that when
// no Origin header is present and the Host header is a remote hostname,
// CheckRequest rejects the request to prevent DNS rebinding attacks.
func TestOriginChecker_CheckRequest_NoOrigin_RemoteHost(t *testing.T) {
	c := NewOriginChecker([]string{"example.com"})
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Host = "evil.com:8080"

	if c.CheckRequest(r) {
		t.Error("CheckRequest should reject remote Host when Origin absent")
	}
}

// TestOriginChecker_CheckRequest_NoOriginNoHost verifies that when neither
// Origin nor Host headers are present, CheckRequest allows the request.
// This handles same-origin requests and tools that don't set these headers.
func TestOriginChecker_CheckRequest_NoOriginNoHost(t *testing.T) {
	c := NewOriginChecker([]string{"example.com"})
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Host = ""

	if !c.CheckRequest(r) {
		t.Error("CheckRequest should allow requests with no origin info")
	}
}

// TestLocalhostOriginChecker verifies that NewLocalhostOriginChecker creates
// a checker that only allows localhost origins — the default behavior for
// local development servers with no explicit allowlist configured.
func TestLocalhostOriginChecker(t *testing.T) {
	c := NewLocalhostOriginChecker()

	// Should allow localhost variants
	for _, origin := range []string{
		"http://localhost:8080",
		"http://127.0.0.1:3000",
		"http://[::1]:9090",
	} {
		if !c.Check(origin) {
			t.Errorf("localhost checker should allow %q", origin)
		}
	}

	// Should reject remote origins
	if c.Check("https://evil.com") {
		t.Error("localhost checker should reject remote origins")
	}
}

// TestLocalhostOriginChecker_CheckRequest verifies that the localhost checker
// works correctly with CheckRequest for Host header fallback.
func TestLocalhostOriginChecker_CheckRequest(t *testing.T) {
	c := NewLocalhostOriginChecker()

	// No origin, localhost host → allow
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Host = "localhost:8080"
	if !c.CheckRequest(r) {
		t.Error("should allow localhost Host")
	}

	// Remote origin → reject
	r2 := httptest.NewRequest("POST", "/mcp", nil)
	r2.Header.Set("Origin", "https://evil.com")
	if c.CheckRequest(r2) {
		t.Error("should reject remote origin")
	}
}

// TestNilOriginChecker_CheckRequest verifies that CheckRequest on a nil
// OriginChecker allows all requests (no-op behavior, matching nil-safe
// middleware pattern).
func TestNilOriginChecker_CheckRequest(t *testing.T) {
	var c *OriginChecker
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Header.Set("Origin", "https://anything.com")

	if !c.CheckRequest(r) {
		t.Error("nil checker should allow all requests")
	}
}
