package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBodyLimiter_NilWhenDisabled verifies that NewBodyLimiter returns nil
// when given a zero or negative max size, following the convention that
// disabled middleware components are represented as nil.
func TestBodyLimiter_NilWhenDisabled(t *testing.T) {
	if bl := NewBodyLimiter(0); bl != nil {
		t.Error("expected nil for maxBytes=0")
	}
	if bl := NewBodyLimiter(-1); bl != nil {
		t.Error("expected nil for maxBytes=-1")
	}
}

// TestBodyLimiter_NilPassthrough verifies that a nil *BodyLimiter's Middleware
// method passes the request through unchanged and returns 200.
func TestBodyLimiter_NilPassthrough(t *testing.T) {
	var bl *BodyLimiter
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := bl.Middleware(inner)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader("hello")))
	if w.Code != 200 {
		t.Errorf("got %d, want 200", w.Code)
	}
}

// TestBodyLimiter_AllowsUnderLimit verifies that a request body smaller than
// the configured limit passes through and the handler can read the full body.
func TestBodyLimiter_AllowsUnderLimit(t *testing.T) {
	bl := NewBodyLimiter(100)
	var readBody string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		readBody = string(data)
		w.WriteHeader(200)
	})
	h := bl.Middleware(inner)

	body := "hello world" // 11 bytes, well under 100
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))

	if w.Code != 200 {
		t.Errorf("got %d, want 200", w.Code)
	}
	if readBody != body {
		t.Errorf("handler read %q, want %q", readBody, body)
	}
}

// TestBodyLimiter_RejectsOverLimit verifies that a request body exceeding
// the configured limit causes a MaxBytesError when the handler reads the body.
func TestBodyLimiter_RejectsOverLimit(t *testing.T) {
	bl := NewBodyLimiter(10)
	var readErr error
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(200)
	})
	h := bl.Middleware(inner)

	body := strings.Repeat("x", 20) // 20 bytes, over limit of 10
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))

	if readErr == nil {
		t.Fatal("expected read error for oversized body")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("got %d, want 413", w.Code)
	}
}

// TestBodyLimiter_ExactLimit verifies that a request body exactly at the
// configured limit passes through without error.
func TestBodyLimiter_ExactLimit(t *testing.T) {
	bl := NewBodyLimiter(5)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if len(data) != 5 {
			http.Error(w, "wrong length", 500)
			return
		}
		w.WriteHeader(200)
	})
	h := bl.Middleware(inner)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader("12345")))
	if w.Code != 200 {
		t.Errorf("got %d, want 200 for exact-limit body", w.Code)
	}
}

// TestBodyLimiter_NoBody verifies that a GET request with no body passes
// through the middleware without error.
func TestBodyLimiter_NoBody(t *testing.T) {
	bl := NewBodyLimiter(100)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := bl.Middleware(inner)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Errorf("got %d, want 200", w.Code)
	}
}
