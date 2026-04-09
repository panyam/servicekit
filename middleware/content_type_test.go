package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRequireContentTypeRejectsWrongType verifies that POST requests with
// a non-matching Content-Type are rejected with 415 Unsupported Media Type.
func TestRequireContentTypeRejectsWrongType(t *testing.T) {
	handler := RequireContentType("application/json")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api", strings.NewReader("key=value"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", rr.Code)
	}
}

// TestRequireContentTypeRejectsMissingType verifies that POST requests
// without a Content-Type header are rejected with 415.
func TestRequireContentTypeRejectsMissingType(t *testing.T) {
	handler := RequireContentType("application/json")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("POST", "/api", strings.NewReader("{}"))
	// No Content-Type set
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415, got %d", rr.Code)
	}
}

// TestRequireContentTypeAcceptsCorrectType verifies that POST requests with
// the correct Content-Type (including charset suffix) pass through.
func TestRequireContentTypeAcceptsCorrectType(t *testing.T) {
	handler := RequireContentType("application/json")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, ct := range []string{"application/json", "application/json; charset=utf-8"} {
		req := httptest.NewRequest("POST", "/api", strings.NewReader("{}"))
		req.Header.Set("Content-Type", ct)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Content-Type %q: expected 200, got %d", ct, rr.Code)
		}
	}
}

// TestRequireContentTypePassesGET verifies that GET requests are not subject
// to Content-Type validation (they typically have no body).
func TestRequireContentTypePassesGET(t *testing.T) {
	handler := RequireContentType("application/json")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for GET, got %d", rr.Code)
	}
}

// TestRequireContentTypePassesDELETE verifies that DELETE requests pass
// through without Content-Type validation.
func TestRequireContentTypePassesDELETE(t *testing.T) {
	handler := RequireContentType("application/json")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("DELETE", "/api", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for DELETE, got %d", rr.Code)
	}
}

// TestRequireContentTypeChecksPUT verifies that PUT requests are also
// subject to Content-Type validation (same as POST).
func TestRequireContentTypeChecksPUT(t *testing.T) {
	handler := RequireContentType("application/json")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("PUT", "/api", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("expected 415 for PUT with wrong Content-Type, got %d", rr.Code)
	}
}
