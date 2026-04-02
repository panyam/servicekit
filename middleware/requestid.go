package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// requestIDKey is the unexported context key type for request IDs.
type requestIDKey struct{}

// RequestID generates or propagates request IDs via the X-Request-Id header.
// If the incoming request has an X-Request-Id header, it is preserved.
// Otherwise, a new 32-character hex string (16 random bytes) is generated.
// The ID is stored in the request context and set on the response header.
type RequestID struct{}

// NewRequestID creates a RequestID middleware instance.
func NewRequestID() *RequestID {
	return &RequestID{}
}

// Middleware returns an HTTP middleware that sets X-Request-Id on request and response.
// On a nil receiver, returns the handler unchanged.
func (rid *RequestID) Middleware(next http.Handler) http.Handler {
	if rid == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = generateID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := ContextWithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext extracts the request ID from context, or "" if absent.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// ContextWithRequestID returns a new context with the given request ID.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// generateID returns a 32-character lowercase hex string from 16 random bytes.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
