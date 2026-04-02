package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthCheck_DefaultPath verifies that a default HealthCheck serves
// on /healthz, returns 200, and responds with {"status":"ok"}.
func TestHealthCheck_DefaultPath(t *testing.T) {
	hc := NewHealthCheck()
	if hc.Path() != "/healthz" {
		t.Errorf("default path = %q, want /healthz", hc.Path())
	}

	w := httptest.NewRecorder()
	hc.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

// TestHealthCheck_CustomPath verifies that WithPath overrides the default
// health check path.
func TestHealthCheck_CustomPath(t *testing.T) {
	hc := NewHealthCheck(WithPath("/ready"))
	if hc.Path() != "/ready" {
		t.Errorf("path = %q, want /ready", hc.Path())
	}
}

// TestHealthCheck_ReadyFuncTrue verifies that when the readiness callback
// returns true, the health check responds with 200.
func TestHealthCheck_ReadyFuncTrue(t *testing.T) {
	hc := NewHealthCheck(WithReadyFunc(func() bool { return true }))

	w := httptest.NewRecorder()
	hc.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestHealthCheck_ReadyFuncFalse verifies that when the readiness callback
// returns false, the health check responds with 503 and {"status":"not ready"}.
func TestHealthCheck_ReadyFuncFalse(t *testing.T) {
	hc := NewHealthCheck(WithReadyFunc(func() bool { return false }))

	w := httptest.NewRecorder()
	hc.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "not ready" {
		t.Errorf("status = %q, want %q", resp["status"], "not ready")
	}
}

// TestHealthCheck_MethodNotAllowed verifies that non-GET requests to the
// health check endpoint receive a 405 Method Not Allowed response.
func TestHealthCheck_MethodNotAllowed(t *testing.T) {
	hc := NewHealthCheck()

	w := httptest.NewRecorder()
	hc.ServeHTTP(w, httptest.NewRequest("POST", "/healthz", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// TestHealthCheck_NilSafe verifies that calling ServeHTTP on a nil
// *HealthCheck does not panic and returns 404.
func TestHealthCheck_NilSafe(t *testing.T) {
	var hc *HealthCheck

	w := httptest.NewRecorder()
	hc.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// TestHealthCheck_ContentType verifies that the health check response
// has Content-Type: application/json.
func TestHealthCheck_ContentType(t *testing.T) {
	hc := NewHealthCheck()

	w := httptest.NewRecorder()
	hc.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
