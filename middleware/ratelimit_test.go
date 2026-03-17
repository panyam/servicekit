package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_PerKey(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		PerKeyPerSec: 1,
		PerKeyBurst:  1,
	})

	// First request for key "a" — allowed
	if !rl.Allow("a") {
		t.Error("first request for key 'a' should be allowed")
	}

	// Second request for same key — rejected
	if rl.Allow("a") {
		t.Error("second request for key 'a' should be rejected")
	}

	// First request for key "b" — allowed (different bucket)
	if !rl.Allow("b") {
		t.Error("first request for key 'b' should be allowed")
	}
}

func TestRateLimiter_GlobalOnly(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		GlobalPerSec: 2,
	})

	// Should allow 2 (burst = GlobalPerSec)
	if !rl.Allow("a") {
		t.Error("first global request should be allowed")
	}
	if !rl.Allow("b") {
		t.Error("second global request should be allowed")
	}

	// Third should be rejected (exceeded burst)
	if rl.Allow("c") {
		t.Error("third request should exceed global limit")
	}
}

func TestRateLimiter_KeyFunc(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		PerKeyPerSec: 1,
		PerKeyBurst:  1,
	})

	// Custom key function that uses a header
	keyFunc := func(r *http.Request) string {
		return r.Header.Get("X-User-ID")
	}

	handler := rl.Middleware(keyFunc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request for user-1
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.Header.Set("X-User-ID", "user-1")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr1.Code)
	}

	// Second request for user-1 — rate limited
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-User-ID", "user-1")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr2.Code)
	}

	// First request for user-2 — allowed
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.Header.Set("X-User-ID", "user-2")
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr3.Code)
	}
}

func TestRateLimiter_OnRejectedWithKey(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		PerKeyPerSec: 1,
		PerKeyBurst:  1,
	})

	var rejectedKey string
	var mu sync.Mutex
	rl.OnRejected = func(key string) {
		mu.Lock()
		rejectedKey = key
		mu.Unlock()
	}

	keyFunc := func(r *http.Request) string {
		return r.Header.Get("X-Key")
	}

	handler := rl.Middleware(keyFunc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request — allowed
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.Header.Set("X-Key", "test-key")
	handler.ServeHTTP(httptest.NewRecorder(), req1)

	// Second request — rejected, should trigger OnRejected
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Key", "test-key")
	handler.ServeHTTP(httptest.NewRecorder(), req2)

	mu.Lock()
	got := rejectedKey
	mu.Unlock()

	if got != "test-key" {
		t.Errorf("OnRejected key: expected 'test-key', got %q", got)
	}
}

func TestRateLimiter_Nil(t *testing.T) {
	var rl *RateLimiter

	// Allow on nil should always return true
	if !rl.Allow("anything") {
		t.Error("nil rate limiter should always allow")
	}

	// Middleware on nil should pass through
	handler := rl.Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("nil rate limiter should pass through, got %d", rr.Code)
	}
}

func TestRateLimiter_NilFromConstructor(t *testing.T) {
	// Both limits zero should return nil
	rl := NewRateLimiter(RateLimitConfig{})
	if rl != nil {
		t.Error("expected nil for zero limits")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{
		PerKeyPerSec:  1,
		PerKeyBurst:   1,
		KeyLimiterTTL: 50 * time.Millisecond,
	})

	// Create entries
	rl.Allow("key-1")
	rl.Allow("key-2")

	rl.mu.Lock()
	count := len(rl.keyLimiters)
	rl.mu.Unlock()
	if count != 2 {
		t.Errorf("expected 2 key limiters, got %d", count)
	}

	// Wait for cleanup
	time.Sleep(150 * time.Millisecond)

	rl.mu.Lock()
	count = len(rl.keyLimiters)
	rl.mu.Unlock()
	if count != 0 {
		t.Errorf("expected 0 key limiters after cleanup, got %d", count)
	}
}

func TestRateLimiter_DefaultKeyFunc(t *testing.T) {
	SetTrustedProxies(nil)

	rl := NewRateLimiter(RateLimitConfig{
		PerKeyPerSec: 1,
		PerKeyBurst:  1,
	})

	// Middleware with nil keyFunc should default to ClientIP
	handler := rl.Middleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "1.2.3.4:5678"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("first request should pass, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "1.2.3.4:5678"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second request from same IP should be limited, got %d", rr2.Code)
	}
}
