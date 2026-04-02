package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// TestRequestID_GeneratesWhenAbsent verifies that when no X-Request-Id header
// is present on the incoming request, the middleware generates a 32-character
// hex string and sets it on the response header.
func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	rid := NewRequestID()
	var gotHeader string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := rid.Middleware(inner)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	gotHeader = w.Header().Get("X-Request-Id")
	if gotHeader == "" {
		t.Fatal("expected X-Request-Id response header, got empty")
	}
	if matched, _ := regexp.MatchString(`^[0-9a-f]{32}$`, gotHeader); !matched {
		t.Errorf("expected 32-char hex string, got %q", gotHeader)
	}
}

// TestRequestID_PropagatesExisting verifies that when the incoming request
// already has an X-Request-Id header, it is preserved on the response and
// injected into the request context unchanged.
func TestRequestID_PropagatesExisting(t *testing.T) {
	rid := NewRequestID()
	existing := "my-trace-id-123"
	var ctxID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = RequestIDFromContext(r.Context())
		w.WriteHeader(200)
	})
	h := rid.Middleware(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", existing)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-Id"); got != existing {
		t.Errorf("response header = %q, want %q", got, existing)
	}
	if ctxID != existing {
		t.Errorf("context ID = %q, want %q", ctxID, existing)
	}
}

// TestRequestID_InContext verifies that the downstream handler can retrieve
// the generated request ID from the request context using RequestIDFromContext.
func TestRequestID_InContext(t *testing.T) {
	rid := NewRequestID()
	var ctxID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = RequestIDFromContext(r.Context())
		w.WriteHeader(200)
	})
	h := rid.Middleware(inner)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))

	if ctxID == "" {
		t.Fatal("expected non-empty request ID in context")
	}
	// Must match the response header
	if respID := w.Header().Get("X-Request-Id"); respID != ctxID {
		t.Errorf("context ID %q != response header %q", ctxID, respID)
	}
}

// TestRequestID_UniquePerRequest verifies that two consecutive requests
// receive different generated request IDs.
func TestRequestID_UniquePerRequest(t *testing.T) {
	rid := NewRequestID()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := rid.Middleware(inner)

	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, httptest.NewRequest("GET", "/", nil))
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, httptest.NewRequest("GET", "/", nil))

	id1 := w1.Header().Get("X-Request-Id")
	id2 := w2.Header().Get("X-Request-Id")
	if id1 == id2 {
		t.Errorf("expected unique IDs, both were %q", id1)
	}
}

// TestRequestID_NilPassthrough verifies that a nil *RequestID's Middleware
// method passes the request through without setting any headers.
func TestRequestID_NilPassthrough(t *testing.T) {
	var rid *RequestID
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := rid.Middleware(inner)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Errorf("got %d, want 200", w.Code)
	}
	if got := w.Header().Get("X-Request-Id"); got != "" {
		t.Errorf("expected no X-Request-Id on nil middleware, got %q", got)
	}
}

// TestRequestID_ContextHelpers verifies that ContextWithRequestID and
// RequestIDFromContext correctly round-trip a request ID value.
func TestRequestID_ContextHelpers(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "test-id-456")
	got := RequestIDFromContext(ctx)
	if got != "test-id-456" {
		t.Errorf("got %q, want %q", got, "test-id-456")
	}
}

// TestRequestID_EmptyContextReturnsEmpty verifies that RequestIDFromContext
// returns an empty string when no request ID has been set in the context.
func TestRequestID_EmptyContextReturnsEmpty(t *testing.T) {
	got := RequestIDFromContext(context.Background())
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
