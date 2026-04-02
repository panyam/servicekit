package middleware

import (
	"net/http"
)

// BodyLimiter limits the size of incoming request bodies using
// http.MaxBytesReader. When the handler reads past the limit,
// MaxBytesReader returns an error and the request fails.
//
// Nil-safe: a nil BodyLimiter passes requests through unchanged.
type BodyLimiter struct {
	maxBytes int64
}

// NewBodyLimiter creates a limiter with the given max body size in bytes.
// Returns nil if maxBytes <= 0 (caller should skip the middleware).
func NewBodyLimiter(maxBytes int64) *BodyLimiter {
	if maxBytes <= 0 {
		return nil
	}
	return &BodyLimiter{maxBytes: maxBytes}
}

// Middleware returns an HTTP middleware that limits request body size.
// On a nil receiver, returns the handler unchanged.
func (b *BodyLimiter) Middleware(next http.Handler) http.Handler {
	if b == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, b.maxBytes)
		next.ServeHTTP(w, r)
	})
}
