package middleware

import (
	"net/http"
)

// HealthCheck is an HTTP handler that serves health/readiness endpoints
// for load balancers and Kubernetes probes. It implements http.Handler
// and should be mounted directly on a mux, not used as middleware.
//
// Nil-safe: a nil *HealthCheck returns 404.
type HealthCheck struct {
	path      string
	readyFunc func() bool
}

// HealthOption configures the health check handler.
type HealthOption func(*HealthCheck)

// WithPath sets the health endpoint path (default "/healthz").
func WithPath(path string) HealthOption {
	return func(h *HealthCheck) {
		h.path = path
	}
}

// WithReadyFunc sets an optional readiness callback. When set and returning
// false, the endpoint returns 503 with {"status":"not ready"}.
func WithReadyFunc(fn func() bool) HealthOption {
	return func(h *HealthCheck) {
		h.readyFunc = fn
	}
}

// NewHealthCheck creates a health check handler with the given options.
// Default path is "/healthz".
func NewHealthCheck(opts ...HealthOption) *HealthCheck {
	h := &HealthCheck{path: "/healthz"}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Path returns the configured endpoint path for mux registration.
func (h *HealthCheck) Path() string {
	if h == nil {
		return ""
	}
	return h.path
}

// ServeHTTP implements http.Handler.
// On a nil receiver, returns 404.
func (h *HealthCheck) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if h.readyFunc != nil && !h.readyFunc() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"not ready"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
